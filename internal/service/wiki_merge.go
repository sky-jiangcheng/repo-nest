package service

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"repo-nest/internal/db"
)

// Merge candidate discovery (ADR-0019 决策 2/3): blocking, not judging.
//
// The compiler only ever writes NEW pending pages and never revises an
// approved one (ADR-0015 决策 3), so a store that lives long enough grows
// near-duplicates: payment-gateway next to gateway-payment, each half-right.
// The ADR's answer is the classic entity-resolution pipeline — blocking →
// matching → merge — with the last step reserved for a human click. This file
// is the first stage only: it surfaces candidate PAIRS and never a conclusion,
// never a write.
//
// Blocking is the engineering point, not matching. Comparing every pair of
// pages is O(n²) work even before anyone spends a token, so candidates come
// from inverted indexes (slug token sets, slug prefixes, title tokens, source
// notes) plus the edge list itself, each bucket capped, and mergeBlocking
// reports how many pair comparisons that cost — TestMergeBlocking_NoFullScan
// pins that the count stays flat as the store grows, which is what "把候选从
// n² 压到 top-K" means in code.
//
// Four signals, all structural (决策 2); none needs lane 2's vectors, so this
// runs today:
//
//   - slug-shape:     normalized slugs share a token set in any order (the
//     word-order split), one prefixes the other, or sit within a typo's edit
//     distance;
//   - shared-source:  the pages cite overlapping source notes (note_pages) —
//     same origin, likely same concept;
//   - title-overlap:  title token Jaccard (CJK-aware bigrams) is high;
//   - cross-linked:   a ref/mentions edge already joins them, yet each tells
//     the story again — the first reader of lane 1's relation edges.
//
// Scoring is layer 1 of 决策 3's two-layer judgment: a weighted sum, and only
// pairs at or above mergeScoreThreshold are worth a model's second look
// (决策 4) or a human's time. Layer 2's optional cosine term over lane 2's
// page_embeddings is absent today — ADR-0018/0021 待决 1 freezes the table name
// but nothing populates it — so it contributes 0 to every pair: absent, not
// faked, and not blocking (决策 3).
//
// What this file deliberately does NOT do: decide two pages are the same, pick
// a winner, or touch the database. db.MergeWikiPages is the only writer, and
// only a human action reaches it (决策 1).

const (
	// mergeTopK bounds the ranked list one pass returns (决策 2's top-K).
	mergeTopK = 50
	// mergeScoreThreshold is 决策 3's layer-1 pass mark.
	mergeScoreThreshold = 0.5
	// mergeSlugEditMax is the Levenshtein budget for the slug-typo signal.
	mergeSlugEditMax = 2
	// mergeSlugPrefixMin: a shorter slug must be at least this long to count as
	// a meaningful prefix ("a" prefixing "a-service" is noise, not a split).
	mergeSlugPrefixMin = 4
	// mergeJaccardMin is the title-token overlap the signal needs.
	mergeJaccardMin = 0.5
	// mergeMaxBucket caps an inverted-index bucket. Past this size the key is a
	// stop word (every page mentions "payment"), and comparing inside it is the
	// n² scan blocking exists to avoid. Oversized buckets are SKIPPED, not
	// truncated: a truncated bucket would make results depend on row order.
	mergeMaxBucket = 60
)

// Signal names. They travel with each candidate so the reviewer — and the
// model's second pass — sees WHY a pair surfaced, not just that it did.
const (
	mergeSignalSlugShape    = "slug-shape"
	mergeSignalSharedSource = "shared-source"
	mergeSignalTitleOverlap = "title-overlap"
	mergeSignalCrossLinked  = "cross-linked"
)

// mergeSignalWeights is layer 1. slug-shape alone clears the threshold because
// payment-gateway/gateway-payment is the flagship split and must not need a
// companion signal to be worth a look. The other three each need company: any
// one of them is common in a healthy store — pages legitimately share a source
// note when one note compiles into several distinct pages (the compile budget
// allows six per note).
var mergeSignalWeights = map[string]float64{
	mergeSignalSlugShape:    0.5,
	mergeSignalSharedSource: 0.25,
	mergeSignalTitleOverlap: 0.25,
	mergeSignalCrossLinked:  0.2,
}

// MergeCandidate is one surfaced pair. A and B are ordered by id so a pair has
// a single canonical form — the dedupe and the idempotent replay both depend
// on it.
type MergeCandidate struct {
	A       db.WikiPage `json:"a"`
	B       db.WikiPage `json:"b"`
	Signals []string    `json:"signals"`
	Score   float64     `json:"score"`
}

// MergeScan is one discovery pass, and is what the binding returns.
type MergeScan struct {
	ProjectID  int64            `json:"project_id"`
	Pages      int              `json:"pages"`
	Pairs      int              `json:"pairs"`     // after blocking, before top-K
	Compared   int              `json:"compared"`  // pair scorings blocking performed
	Candidates []MergeCandidate `json:"candidates"`
	// Layer2Note records 决策 3's optional second layer. Lane 2's
	// page_embeddings does not exist yet, so the cosine term is 0 for every
	// pair and the threshold filter is the whole judgment; keeping the absence
	// visible beats a silently missing term.
	Layer2Note string `json:"layer2_note,omitempty"`
}

// DiscoverMergeCandidates runs one blocking pass over projectID (0 = every
// page). It reads the store and nothing else: no model, no vectors, no writes.
func (s *Service) DiscoverMergeCandidates(projectID int64) (*MergeScan, error) {
	pages, err := db.ListWikiPages(s.db, "", projectID)
	if err != nil {
		return nil, err
	}
	scan := &MergeScan{
		ProjectID:  projectID,
		Pages:      len(pages),
		Candidates: []MergeCandidate{},
		Layer2Note: "lane 2 未落地（page_embeddings 不存在）：第二层余弦项恒 0，判定以第一层打分为准",
	}
	eligible := make([]db.WikiPage, 0, len(pages))
	for _, p := range pages {
		// A rejected page is already disposed of (a merge tombstones its
		// loser); re-surfacing one as a candidate is noise.
		if p.Status == db.WikiStatusRejected {
			continue
		}
		eligible = append(eligible, p)
	}
	// Prefetch per page, once: asking inside the pair loop would turn the
	// blocking indexes into a query per comparison (the lintMissingRefs lesson).
	notes := make(map[int64][]int64, len(eligible))
	for _, p := range eligible {
		ns, err := db.NotesForPage(s.db, p.ID)
		if err != nil {
			return nil, fmt.Errorf("db: notes for page %d: %w", p.ID, err)
		}
		notes[p.ID] = ns
	}
	edges, err := db.ListPageLinks(s.db)
	if err != nil {
		return nil, err
	}

	pairs, compared := mergeBlocking(eligible, notes, edges)
	scan.Compared = compared
	scan.Pairs = len(pairs)

	byID := make(map[int64]db.WikiPage, len(eligible))
	for _, p := range eligible {
		byID[p.ID] = p
	}
	linked := mergeLinkedPairs(edges)
	scored := make([]MergeCandidate, 0, len(pairs))
	for _, pair := range pairs {
		a, okA := byID[pair[0]]
		b, okB := byID[pair[1]]
		if !okA || !okB {
			continue
		}
		if c, ok := scoreMergePair(a, b, notes, linked); ok {
			scored = append(scored, c)
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		// Deterministic below the score: two runs over an unchanged store must
		// return the same list (the same reason ListPageVersions tiebreaks on id).
		if scored[i].A.ID != scored[j].A.ID {
			return scored[i].A.ID < scored[j].A.ID
		}
		return scored[i].B.ID < scored[j].B.ID
	})
	if len(scored) > mergeTopK {
		scored = scored[:mergeTopK]
	}
	scan.Candidates = append(scan.Candidates, scored...)
	return scan, nil
}

// MergeWikiPages folds loser into winner (决策 5). Human-triggered by contract:
// the review UI's adopt button is the only caller — no compiler, lint, model or
// job path may reach it (决策 1). db.MergeWikiPages holds the transaction's
// guarantees (edges re-pointed, provenance moved, winner untouched).
func (s *Service) MergeWikiPages(winnerID, loserID int64) error {
	return db.MergeWikiPages(s.db, winnerID, loserID)
}

// MergeCandidatesAboveThreshold filters to the pairs that cleared 决策 3's
// layer-1 mark — the only ones worth the model's second judgment (决策 4) or a
// human's time. Candidates below it stay visible in the scan for a human
// browsing, but no automated path may spend money on them.
func MergeCandidatesAboveThreshold(cands []MergeCandidate) []MergeCandidate {
	out := make([]MergeCandidate, 0, len(cands))
	for _, c := range cands {
		if c.Score >= mergeScoreThreshold {
			out = append(out, c)
		}
	}
	return out
}

// mergeBlocking turns the store into candidate pairs through inverted indexes,
// never an all-pairs loop. It returns the pairs (canonical: smaller id first,
// deduplicated) and the number of pair comparisons the buckets cost — the
// counter is what TestMergeBlocking_NoFullScan asserts stays flat.
func mergeBlocking(pages []db.WikiPage, notes map[int64][]int64, edges []db.PageLinkRow) ([][2]int64, int) {
	byID := make(map[int64]db.WikiPage, len(pages))
	for _, p := range pages {
		byID[p.ID] = p
	}
	// 决策 4: no cross-project merges in the MVP. Checked at emission so a pair
	// the merge itself would refuse never becomes a candidate at all.
	pairable := func(a, b db.WikiPage) bool {
		return a.ProjectID == b.ProjectID || a.ProjectID == 0 || b.ProjectID == 0
	}
	seen := map[[2]int64]bool{}
	var pairs [][2]int64
	compared := 0
	emit := func(aID, bID int64) {
		if aID == bID {
			return
		}
		if aID > bID {
			aID, bID = bID, aID
		}
		key := [2]int64{aID, bID}
		if seen[key] {
			return
		}
		a, okA := byID[aID]
		b, okB := byID[bID]
		if !okA || !okB || !pairable(a, b) {
			return
		}
		seen[key] = true
		pairs = append(pairs, key)
	}
	// bucket compares every pair inside one inverted-index bucket, counting the
	// work. Oversized buckets are skipped wholesale (see mergeMaxBucket).
	bucket := func(ids []int64, signal func(aID, bID int64) bool) {
		if len(ids) < 2 || len(ids) > mergeMaxBucket {
			return
		}
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				compared++
				if signal(ids[i], ids[j]) {
					emit(ids[i], ids[j])
				}
			}
		}
	}

	// Slug buckets: exact token-set key (catches word-order splits without any
	// comparison) and a 4-char prefix key (catches prefix relations and typos,
	// which mergeSlugShape then verifies exactly).
	tokenBuckets := map[string][]int64{}
	prefixBuckets := map[string][]int64{}
	for _, p := range pages {
		slug := db.NormalizeWikiSlug(p.Slug)
		if slug == "" {
			continue
		}
		if key := strings.Join(sortedTokens(slug), "-"); key != "" {
			tokenBuckets[key] = append(tokenBuckets[key], p.ID)
		}
		if len(slug) >= mergeSlugPrefixMin {
			prefixBuckets[slug[:mergeSlugPrefixMin]] = append(prefixBuckets[slug[:mergeSlugPrefixMin]], p.ID)
		}
	}
	slugShape := func(aID, bID int64) bool {
		return mergeSlugShape(byID[aID], byID[bID])
	}
	for _, ids := range tokenBuckets {
		bucket(ids, slugShape)
	}
	for _, ids := range prefixBuckets {
		bucket(ids, slugShape)
	}

	// Title-token buckets: one entry per token, so a pair is compared once per
	// shared token and only the Jaccard check decides.
	titleTokens := make(map[int64]map[string]bool, len(pages))
	tokenIdx := map[string][]int64{}
	for _, p := range pages {
		toks := mergeTitleTokens(p.Title)
		titleTokens[p.ID] = toks
		for t := range toks {
			tokenIdx[t] = append(tokenIdx[t], p.ID)
		}
	}
	for _, ids := range tokenIdx {
		bucket(ids, func(aID, bID int64) bool {
			return jaccard(titleTokens[aID], titleTokens[bID]) >= mergeJaccardMin
		})
	}

	// Source-note buckets: sharing a note IS the signal, so the bucket needs no
	// comparison beyond membership.
	noteIdx := map[int64][]int64{}
	for pageID, ns := range notes {
		for _, n := range ns {
			noteIdx[n] = append(noteIdx[n], pageID)
		}
	}
	for _, ids := range noteIdx {
		bucket(ids, func(_, _ int64) bool { return true })
	}

	// The edge list is itself a blocking index: a ref/mentions edge is a pair
	// with no comparison needed (which is why it costs nothing in `compared`).
	for pair := range mergeLinkedPairs(edges) {
		emit(pair[0], pair[1])
	}
	return pairs, compared
}

// mergeLinkedPairs indexes the ref/mentions edges — the pages that already
// point at each other. Reading lane 1's relation edges is the first consumer of
// the vocabulary ADR-0018 froze; the other six relations say nothing about
// sameness (a depends edge between two pages is the normal case).
func mergeLinkedPairs(edges []db.PageLinkRow) map[[2]int64]bool {
	out := map[[2]int64]bool{}
	for _, e := range edges {
		if e.Relation != db.RelationRef && e.Relation != db.RelationMentions {
			continue
		}
		a, b := e.FromID, e.ToID
		if a > b {
			a, b = b, a
		}
		out[[2]int64{a, b}] = true
	}
	return out
}

// scoreMergePair recomputes the four signals for one pair and weights them
// (决策 3, layer 1). ok=false means no signal fired: a bucket proposed the
// pair, the signals declined it.
func scoreMergePair(a, b db.WikiPage, notes map[int64][]int64, linked map[[2]int64]bool) (MergeCandidate, bool) {
	var signals []string
	if mergeSlugShape(a, b) {
		signals = append(signals, mergeSignalSlugShape)
	}
	if mergeSharedSource(notes, a.ID, b.ID) {
		signals = append(signals, mergeSignalSharedSource)
	}
	if jaccard(mergeTitleTokens(a.Title), mergeTitleTokens(b.Title)) >= mergeJaccardMin {
		signals = append(signals, mergeSignalTitleOverlap)
	}
	key := [2]int64{a.ID, b.ID}
	if a.ID > b.ID {
		key = [2]int64{b.ID, a.ID}
	}
	if linked[key] {
		signals = append(signals, mergeSignalCrossLinked)
	}
	if len(signals) == 0 {
		return MergeCandidate{}, false
	}
	score := 0.0
	for _, s := range signals {
		score += mergeSignalWeights[s]
	}
	sort.Strings(signals) // stable order regardless of which check ran first
	return MergeCandidate{A: a, B: b, Signals: signals, Score: score}, true
}

// mergeSlugShape reports the slug-morphology signal: the same token set in any
// order (payment-gateway / gateway-payment), one slug prefixing the other
// (payment / payment-gateway), or a typo's edit distance apart.
func mergeSlugShape(a, b db.WikiPage) bool {
	sa, sb := db.NormalizeWikiSlug(a.Slug), db.NormalizeWikiSlug(b.Slug)
	if sa == "" || sb == "" {
		return false
	}
	if sa == sb {
		return true
	}
	if strings.Join(sortedTokens(sa), "-") == strings.Join(sortedTokens(sb), "-") {
		return true
	}
	shorter, longer := sa, sb
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(shorter) >= mergeSlugPrefixMin && strings.HasPrefix(longer, shorter) {
		return true
	}
	if d, ok := levenshteinWithin(sa, sb, mergeSlugEditMax); ok && d > 0 {
		return true
	}
	return false
}

// mergeSharedSource reports whether the two pages cite an overlapping set of
// source notes. Same origin is evidence of the same concept — and when it is
// not (one note compiling into several distinct pages), the weight alone keeps
// the pair below the threshold until a second signal agrees.
func mergeSharedSource(notes map[int64][]int64, aID, bID int64) bool {
	for _, n := range notes[aID] {
		for _, m := range notes[bID] {
			if n == m {
				return true
			}
		}
	}
	return false
}

// levenshteinWithin computes the edit distance between a and b, giving up as
// soon as it exceeds max. The give-up is the point: an unbounded Levenshtein
// on every same-prefix pair is how a cheap blocking step quietly becomes the
// expensive one.
func levenshteinWithin(a, b string, max int) (int, bool) {
	if a == b {
		return 0, true
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(b)-len(a) > max {
		return 0, false
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		best := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			best = min(best, cur[j])
		}
		if best > max {
			return 0, false
		}
		prev, cur = cur, prev
	}
	if d := prev[len(b)]; d <= max {
		return d, true
	}
	return 0, false
}

// mergeTitleTokens splits a title into comparison tokens: ASCII words, plus
// character bigrams for CJK runs. Without the bigram half, two Chinese titles
// one character apart ("支付网关" vs "支付网关卡") share zero tokens and the
// signal could never fire on the store's dominant language.
func mergeTitleTokens(title string) map[string]bool {
	out := map[string]bool{}
	var word strings.Builder
	var cjk []rune
	flushWord := func() {
		if word.Len() > 0 {
			out[word.String()] = true
			word.Reset()
		}
	}
	flushCJK := func() {
		if len(cjk) == 1 {
			out[string(cjk)] = true
		}
		for i := 0; i+1 < len(cjk); i++ {
			out[string(cjk[i:i+2])] = true
		}
		cjk = cjk[:0]
	}
	for _, r := range strings.ToLower(title) {
		switch {
		case isHanRune(r):
			flushWord()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			word.WriteRune(r)
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return out
}

// isHanRune covers the CJK ideograph ranges a Chinese title is made of; kana
// and hangul tokenize as ordinary letters, which is good enough for a title
// overlap heuristic.
func isHanRune(r rune) bool {
	return r >= 0x3400 && r <= 0x4dbf || r >= 0x4e00 && r <= 0x9fff || r >= 0xf900 && r <= 0xfaff
}

// jaccard is the token-set overlap, 0 when either side is empty (an empty
// title must not make every pair look similar).
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for t := range a {
		if b[t] {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}

// sortedTokens splits a normalized slug on hyphens and sorts the parts, so
// payment-gateway and gateway-payment produce the same key.
func sortedTokens(s string) []string {
	parts := strings.Split(s, "-")
	toks := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			toks = append(toks, p)
		}
	}
	sort.Strings(toks)
	return toks
}
