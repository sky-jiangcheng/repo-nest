package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// snapshotSchema records every object sqlite knows about, so a migration can be
// proven reversible rather than merely "not crashing".
func snapshotSchema(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT type, name, COALESCE(sql, '') FROM sqlite_master ORDER BY type, name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var typ, name, sql string
		if err := rows.Scan(&typ, &name, &sql); err != nil {
			t.Fatal(err)
		}
		out = append(out, typ+"|"+name+"|"+sql)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// W1's acceptance bar from ADR-0014: "迁移必须可逆并带测试".
//
// Reversible here means something stronger than "there is a down script": the
// wiki layer adds only new objects and touches no pre-existing table, so after
// DropWikiSchema the schema must be IDENTICAL to before. Anything less would let
// a future "small" ALTER into project_notes hide inside this migration.
func TestWikiMigration_IsReversibleByConstruction(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "rev.db")

	db1, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// v16 has already run, so capture "before" at v15 by dropping the layer, then
	// re-apply and compare. This also proves EnsureWikiSchema is replay-safe.
	before := func() []string {
		if err := DropWikiSchema(db1); err != nil {
			t.Fatalf("drop: %v", err)
		}
		return snapshotSchema(t, db1)
	}()
	if err := EnsureWikiSchema(db1); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	dirty := snapshotSchema(t, db1)
	if reflect.DeepEqual(before, dirty) {
		t.Fatal("schema unchanged after EnsureWikiSchema — the layer created nothing, so the comparison is vacuous")
	}
	if err := DropWikiSchema(db1); err != nil {
		t.Fatalf("drop again: %v", err)
	}
	after := snapshotSchema(t, db1)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("wiki layer is not reversible:\n before=%d objects\n after =%d objects\n diff:\n  %v\n  %v",
			len(before), len(after), minus(before, after), minus(after, before))
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	// A fresh database must arrive with the layer present (migration ran, not just
	// a helper someone might forget to call).
	db2, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db2.Close() }()
	if _, err := db2.Query("SELECT 1 FROM wiki_pages LIMIT 1"); err != nil {
		t.Fatalf("fresh InitDB has no wiki_pages: %v", err)
	}
}

func minus(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	var out []string
	for _, x := range a {
		if !set[x] {
			out = append(out, x)
		}
	}
	return out
}

func TestWikiPages_KindIsEnforced(t *testing.T) {
	db := setupTestDB(t)

	if _, err := CreateWikiPage(db, "nonsense", "orphan-page", "Orphan", 0, ""); err == nil {
		t.Fatal("an unknown kind was accepted by CreateWikiPage")
	}
	// The CHECK must hold even if someone bypasses the Go validator, otherwise the
	// constraint is decoration and the enum lives in one place only.
	if _, err := db.Exec(
		`INSERT INTO wiki_pages(slug, title, kind) VALUES ('raw-insert','Raw','who-knows')`); err == nil {
		t.Error("the CHECK constraint did not reject an unknown kind at the SQL level")
	}
	for _, k := range []string{WikiKindEntity, WikiKindConcept, WikiKindSource, WikiKindSynthesis, WikiKindQuery} {
		if _, err := CreateWikiPage(db, k, "page-"+k, "Page "+k, 0, "body"); err != nil {
			t.Errorf("kind %q rejected: %v", k, err)
		}
	}
	counts, err := WikiPageCount(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 5 {
		t.Errorf("kind histogram = %v, want five distinct kinds", counts)
	}
}

// The slug is the wikilink address, so uniqueness is the one thing W2/W3 will
// depend on absolutely — and the reason it is globally unique rather than per
// project (a per-project index would leave a NULL hole for cross-project pages).
func TestWikiPages_SlugIsGloballyUniqueAndDistinguishable(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "slugproj")

	if _, err := CreateWikiPage(db, WikiKindEntity, "payment-gateway", "Payment Gateway", pid, "first"); err != nil {
		t.Fatal(err)
	}
	_, err := CreateWikiPage(db, WikiKindConcept, "payment-gateway", "Duplicate", 0, "second")
	if err == nil {
		t.Fatal("a duplicate slug was accepted")
	}
	if !errors.Is(err, ErrWikiSlugTaken) {
		t.Errorf("duplicate slug error = %v, want it to wrap ErrWikiSlugTaken so callers can tell conflict from failure", err)
	}

	// Normalization must be deterministic and shared with link resolution.
	cases := map[string]string{
		"Payment Gateway":        "payment-gateway",
		"  Multi   Space  ":      "multi-space",
		"Under_score.dot":        "under-score-dot",
		"中文 标题":                  "中文-标题",
		"trailing----":           "trailing",
		"CamelCase Name":         "camelcase-name",
		"---leading and tail---": "leading-and-tail",
	}
	for in, want := range cases {
		if got := NormalizeWikiSlug(in); got != want {
			t.Errorf("NormalizeWikiSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

// The whole point of the layer over an adjacency list is the backlink direction.
func TestWikiLinks_AreBidirectionalAndIdempotent(t *testing.T) {
	db := setupTestDB(t)

	a, err := CreateWikiPage(db, WikiKindEntity, "auth-module", "Auth Module", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateWikiPage(db, WikiKindConcept, "session-timeout", "Session Timeout", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, b.ID, a.ID, RelationImplements); err != nil {
		t.Fatal(err)
	}
	// Same triple twice: a re-run of the same ingest must not stack edges.
	if err := LinkWikiPages(db, b.ID, a.ID, RelationImplements); err != nil {
		t.Fatalf("re-link was rejected: %v", err)
	}
	// A different relation between the same pair IS a different edge.
	if err := LinkWikiPages(db, b.ID, a.ID, RelationDepends); err != nil {
		t.Fatal(err)
	}

	out, err := WikiEdgesFrom(db, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("outgoing edges = %d, want 2 (one per relation)", len(out))
	}
	back, err := WikiEdgesTo(db, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 {
		t.Fatalf("backlinks = %d, want 2", len(back))
	}
	for _, e := range back {
		if !e.Inbound {
			t.Error("backlink rows are not marked inbound")
		}
		if e.PageID != b.ID {
			t.Errorf("backlink points at %d, want %d", e.PageID, b.ID)
		}
	}

	if err := UnlinkWikiPages(db, b.ID, a.ID, RelationImplements); err != nil {
		t.Fatal(err)
	}
	after, err := WikiEdgesTo(db, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].Relation != RelationDepends {
		t.Errorf("after unlink: %+v, want only %s", after, RelationDepends)
	}
}

func TestWikiLinks_RejectSelfLoopsAndDanglingEndpoints(t *testing.T) {
	db := setupTestDB(t)
	a, err := CreateWikiPage(db, WikiKindEntity, "solo", "Solo", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, a.ID, a.ID, "ref"); err == nil {
		t.Error("a page was allowed to link to itself")
	}
	if err := LinkWikiPages(db, a.ID, 9999, "ref"); err == nil {
		t.Error("a link to a nonexistent page was accepted — dangling edges would make W4 lint meaningless")
	}
}

// Deleting a page must not leave edges or note attachments behind: orphan rows in
// a link table look harmless until they surface as a page with a ghost neighbor.
func TestWikiPages_DeleteCascades(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "cascadeproj")
	note, err := CreateNoteEx(db, pid, "note", "body", "", "knowledge", "manual")
	if err != nil {
		t.Fatal(err)
	}
	from, err := CreateWikiPage(db, WikiKindSource, "src-a", "Src A", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	to, err := CreateWikiPage(db, WikiKindSynthesis, "syn-b", "Syn B", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, from.ID, to.ID, "feeds"); err != nil {
		t.Fatal(err)
	}
	if err := AttachNoteToPage(db, note.ID, from.ID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteWikiPage(db, from.ID); err != nil {
		t.Fatal(err)
	}

	var edges, attachments int
	if err := db.QueryRow("SELECT COUNT(*) FROM page_links").Scan(&edges); err != nil {
		t.Fatal(err)
	}
	if edges != 0 {
		t.Errorf("%d edge(s) survived deleting a page", edges)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM note_pages").Scan(&attachments); err != nil {
		t.Fatal(err)
	}
	if attachments != 0 {
		t.Errorf("%d note attachment(s) survived deleting a page", attachments)
	}
}

// The second structural gap project_notes never closed: a note could name a
// project but never the repository inside it.
func TestNoteRepositoryAssociation(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "repos")
	repoPath := "/tmp/repos/sub"
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertRepositoryTx(tx, repoPath, pid); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var repoID int64
	if err := db.QueryRow("SELECT id FROM repositories WHERE path = ?", repoPath).Scan(&repoID); err != nil {
		t.Fatal(err)
	}
	note, err := CreateNoteEx(db, pid, "note", "body", "", "other", "manual")
	if err != nil {
		t.Fatal(err)
	}

	if err := AttachNoteToRepository(db, note.ID, repoID); err != nil {
		t.Fatal(err)
	}
	if err := AttachNoteToRepository(db, note.ID, repoID); err != nil {
		t.Fatalf("re-attach rejected (must be idempotent): %v", err)
	}
	ids, err := RepositoriesForNote(db, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != repoID {
		t.Errorf("RepositoriesForNote = %v, want [%d]", ids, repoID)
	}
	back, err := NotesForRepository(db, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 || back[0] != note.ID {
		t.Errorf("NotesForRepository = %v, want [%d]", back, note.ID)
	}
	if err := DetachNoteFromRepository(db, note.ID, repoID); err != nil {
		t.Fatal(err)
	}
	if got, _ := RepositoriesForNote(db, note.ID); len(got) != 0 {
		t.Errorf("after detach: %v, want empty", got)
	}
	// Deleting the note must clean up its side of the association.
	if err := AttachNoteToRepository(db, note.ID, repoID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteNote(db, note.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow("SELECT COUNT(*) FROM note_repositories").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d note/repository row(s) survived deleting the note", left)
	}
}

// A page attached to a project must disappear with the project, the same cascade
// the notes it was compiled from already follow.
func TestWikiPages_ProjectDeleteCascades(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "cascade-owner")
	if _, err := CreateWikiPage(db, WikiKindEntity, "owned-page", "Owned", pid, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM projects WHERE id = ?", pid); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM wiki_pages WHERE project_id = ?", pid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d page(s) survived deleting their project", n)
	}
}

// ADR-0018 lane 2: the one-hop walk evidence retrieval performs. The closed
// set of walkable relations, the approved-only gate, and the scope rule are
// all load-bearing: contradicts is the exclusion that matters most, because a
// conflict is something to show the user, not to blend into the same evidence
// block as the claim it conflicts with.
func TestWikiNeighbors_OneHopBothDirections(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "neigh")

	seed, err := CreateWikiPage(db, WikiKindEntity, "seed", "Seed", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	outN, err := CreateWikiPage(db, WikiKindEntity, "out-neighbor", "Out Neighbor", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	inN, err := CreateWikiPage(db, WikiKindEntity, "in-neighbor", "In Neighbor", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	far, err := CreateWikiPage(db, WikiKindEntity, "two-hops", "Two Hops", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, seed.ID, outN.ID, RelationRef); err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, inN.ID, seed.ID, RelationRef); err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, outN.ID, far.ID, RelationRef); err != nil {
		t.Fatal(err)
	}

	got, err := WikiNeighbors(db, []int64{seed.ID}, pid, 10)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	for _, n := range got {
		ids[n.Page.ID] = true
	}
	if !ids[outN.ID] || !ids[inN.ID] {
		t.Errorf("neighbors = %v, want both the outbound and the inbound page", ids)
	}
	if ids[seed.ID] {
		t.Error("a seed was returned as its own neighbor")
	}
	if ids[far.ID] {
		t.Error("a page two hops away was returned; the walk must stop at one hop")
	}

	// A second relation between the same pair is a different edge but the same
	// neighbor: one row per page, never one per edge.
	if err := LinkWikiPages(db, seed.ID, outN.ID, RelationPartOf); err != nil {
		t.Fatal(err)
	}
	again, err := WikiNeighbors(db, []int64{seed.ID}, pid, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(got) {
		t.Errorf("after adding a parallel edge: %d neighbors, want still %d", len(again), len(got))
	}
}

func TestWikiNeighbors_TraversalWhitelist(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "whitelist")

	seed, err := CreateWikiPage(db, WikiKindEntity, "seed", "Seed", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	// One target per relation, so the result set names exactly the walkable set.
	byRelation := map[string]string{
		RelationRef:         "by-ref",
		RelationPartOf:      "by-part-of",
		RelationDocuments:   "by-documents",
		RelationDepends:     "by-depends",
		RelationImplements:  "by-implements",
		RelationSupersedes:  "by-supersedes",
		RelationContradicts: "by-contradicts",
		RelationMentions:    "by-mentions",
	}
	for rel, slug := range byRelation {
		p, err := CreateWikiPage(db, WikiKindEntity, slug, "T "+slug, pid, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := LinkWikiPages(db, seed.ID, p.ID, rel); err != nil {
			t.Fatal(err)
		}
	}

	got, err := WikiNeighbors(db, []int64{seed.ID}, pid, 50)
	if err != nil {
		t.Fatal(err)
	}
	slugs := map[string]bool{}
	for _, n := range got {
		slugs[n.Page.Slug] = true
	}
	for _, want := range []string{"by-ref", "by-part-of", "by-documents"} {
		if !slugs[want] {
			t.Errorf("walkable relation target %q missing from %v", want, slugs)
		}
	}
	for _, forbidden := range []string{
		"by-depends", "by-implements", "by-supersedes", "by-contradicts", "by-mentions",
	} {
		if slugs[forbidden] {
			t.Errorf("%q reached evidence through a non-walkable relation", forbidden)
		}
	}
}

func TestWikiNeighbors_OnlyApprovedAndScoped(t *testing.T) {
	db := setupTestDB(t)
	home := createTestProject(t, db, "home")
	other := createTestProject(t, db, "other")

	seed, err := CreateWikiPage(db, WikiKindEntity, "seed", "Seed", home, "")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := CreateWikiPageAs(db, WikiKindEntity, "pending-n", "Pending", home, "",
		WikiStatusPending, "wiki-compile")
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := CreateWikiPageAs(db, WikiKindEntity, "rejected-n", "Rejected", home, "",
		WikiStatusRejected, "wiki-compile")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := CreateWikiPage(db, WikiKindEntity, "foreign-n", "Foreign", other, "")
	if err != nil {
		t.Fatal(err)
	}
	global, err := CreateWikiPage(db, WikiKindConcept, "global-n", "Global", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []*WikiPage{pending, rejected, foreign, global} {
		if err := LinkWikiPages(db, seed.ID, p.ID, RelationRef); err != nil {
			t.Fatal(err)
		}
	}

	got, err := WikiNeighbors(db, []int64{seed.ID}, home, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range got {
		switch n.Page.ID {
		case pending.ID, rejected.ID:
			t.Errorf("%s page entered the neighbor set; only approved pages may answer", n.Page.Status)
		case foreign.ID:
			t.Error("another project's page entered a project-scoped walk")
		}
	}
	var sawGlobal bool
	for _, n := range got {
		if n.Page.ID == global.ID {
			sawGlobal = true
		}
	}
	if !sawGlobal {
		t.Error("a global page was excluded from the walk; global pages belong everywhere")
	}
}

func TestWikiNeighbors_InlinksAndLimit(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "inlinks")

	seed, err := CreateWikiPage(db, WikiKindEntity, "seed", "Seed", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	popular, err := CreateWikiPage(db, WikiKindEntity, "popular", "Popular", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := CreateWikiPage(db, WikiKindEntity, "quiet", "Quiet", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	// Both are one hop from the seed; popular also carries three inbound edges
	// from consumer pages that are NOT themselves neighbors of the seed. The
	// tally is the page's full in-degree, so the seed's own edge counts too:
	// popular has 4, quiet has 1.
	for i := 0; i < 3; i++ {
		c, err := CreateWikiPage(db, WikiKindConcept, "consumer-"+string(rune('a'+i)), "C", pid, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := LinkWikiPages(db, c.ID, popular.ID, RelationRef); err != nil {
			t.Fatal(err)
		}
	}
	if err := LinkWikiPages(db, seed.ID, popular.ID, RelationRef); err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, seed.ID, quiet.ID, RelationRef); err != nil {
		t.Fatal(err)
	}

	got, err := WikiNeighbors(db, []int64{seed.ID}, pid, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("neighbors = %d, want 2", len(got))
	}
	if got[0].Page.ID != popular.ID {
		t.Errorf("first neighbor = %d, want the higher in-degree page %d", got[0].Page.ID, popular.ID)
	}
	if got[0].Inlinks != 4 || got[1].Inlinks != 1 {
		t.Errorf("inlinks = %d/%d, want 4/1", got[0].Inlinks, got[1].Inlinks)
	}

	one, err := WikiNeighbors(db, []int64{seed.ID}, pid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].Page.ID != popular.ID {
		t.Errorf("limit 1 returned %+v, want just the popular page", one)
	}
	// No seeds is not an error: an empty FTS result must not become a query.
	none, err := WikiNeighbors(db, nil, pid, 10)
	if err != nil || len(none) != 0 {
		t.Errorf("empty seeds = (%v, %v), want (nil, nil)", none, err)
	}
}

// WikiWalkEdges is the edge half of the PPR walk (ADR-0018 lane 2): the
// walkable edges with BOTH endpoints inside the candidate set, one row per
// edge. Edges leaving the set are not the walk's business — their mass
// becomes dangling inside the induced subgraph — and a relation outside the
// whitelist is never walked, contradicts above all.
func TestWikiWalkEdges_WhitelistAndBothEndpoints(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "walk-edges")

	a, err := CreateWikiPage(db, WikiKindEntity, "a", "A", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateWikiPage(db, WikiKindEntity, "b", "B", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := CreateWikiPage(db, WikiKindEntity, "c", "C", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	outside, err := CreateWikiPage(db, WikiKindEntity, "outside", "Outside", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []struct {
		from, to int64
		relation string
	}{
		{a.ID, b.ID, RelationRef},
		{b.ID, c.ID, RelationPartOf},
		{a.ID, c.ID, RelationContradicts},
		{c.ID, outside.ID, RelationDocuments},
	} {
		if err := LinkWikiPages(db, e.from, e.to, e.relation); err != nil {
			t.Fatal(err)
		}
	}

	got, err := WikiWalkEdges(db, []int64{a.ID, b.ID, c.ID})
	if err != nil {
		t.Fatal(err)
	}
	type edge struct{ from, to int64 }
	set := map[edge]int{}
	for _, e := range got {
		set[edge{e.From, e.To}]++
	}
	if set[edge{a.ID, b.ID}] != 1 || set[edge{b.ID, c.ID}] != 1 {
		t.Errorf("walk edges = %v, want a->b and b->c exactly once each", set)
	}
	if set[edge{a.ID, c.ID}] != 0 {
		t.Error("a contradicts edge was returned; the whitelist must hold in SQL too")
	}
	if set[edge{c.ID, outside.ID}] != 0 {
		t.Error("an edge leaving the candidate set was returned; both endpoints must be inside")
	}
}

// Two walkable relations between the same pair are two edges: multiplicity is
// the only weight the walk has, and collapsing them here would silently
// reweight the graph.
func TestWikiWalkEdges_ParallelRelationsCountSeparately(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "walk-parallel")

	a, err := CreateWikiPage(db, WikiKindEntity, "a", "A", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateWikiPage(db, WikiKindEntity, "b", "B", pid, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, a.ID, b.ID, RelationRef); err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, a.ID, b.ID, RelationDocuments); err != nil {
		t.Fatal(err)
	}

	got, err := WikiWalkEdges(db, []int64{a.ID, b.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("walk edges = %+v, want the parallel pair as two rows", got)
	}
}

// ADR-0018 lane 1: the relation CHECK is live, and NormalizeRelation is the
// single gate every writer passes through.
func TestRelationVocabulary_CheckIsEnforced(t *testing.T) {
	db := setupTestDB(t)
	a, err := CreateWikiPage(db, WikiKindEntity, "a", "A", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateWikiPage(db, WikiKindConcept, "b", "B", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	// A raw insert that bypasses the Go gate must be refused by the CHECK — that
	// is the whole point of v21: no writer can plant an off-vocabulary relation.
	if _, err := db.Exec(
		`INSERT INTO page_links(from_page_id, to_page_id, relation) VALUES (?,?,?)`,
		a.ID, b.ID, "nonsense"); err == nil {
		t.Fatal("CHECK accepted an off-vocabulary relation")
	}
	if err := LinkWikiPages(db, a.ID, b.ID, RelationDepends); err != nil {
		t.Fatalf("LinkWikiPages(valid) rejected: %v", err)
	}
	// The API gate maps a legacy string onto the vocabulary instead of erroring,
	// so a mislabelled edge degrades to ref rather than blocking a good write.
	if err := LinkWikiPages(db, a.ID, b.ID, "compiled-from"); err != nil {
		t.Fatalf("LinkWikiPages(alias) rejected: %v", err)
	}
	var got []string
	rows, err := db.Query(
		`SELECT relation FROM page_links WHERE from_page_id=? AND to_page_id=? ORDER BY relation`, a.ID, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if len(got) != 2 || got[0] != RelationDepends || got[1] != RelationRef {
		t.Errorf("edges = %v, want [%s %s]", got, RelationDepends, RelationRef)
	}
}

func TestNormalizeRelation(t *testing.T) {
	cases := map[string]string{
		"":               RelationRef,
		"ref":            RelationRef,
		"REF":            RelationRef,
		"  depends ":     RelationDepends,
		"depends-on":     RelationDepends, // legacy spelling alias
		"compiled-from":  RelationRef,     // old compiler default
		"cites":          RelationRef,     // old filing default
		"part_of":        RelationPartOf,
		"contradicts":    RelationContradicts,
		"total-nonsense": RelationRef, // unknown → ref
		"mentions":       RelationMentions,
	}
	for in, want := range cases {
		if got := NormalizeRelation(in); got != want {
			t.Errorf("NormalizeRelation(%q) = %q, want %q", in, got, want)
		}
	}
}

// The CHECK is rendered from wikiRelations, but guard against silent drift: parse
// the IN-list out of the DDL and require it to match the Go set exactly.
func TestRelationVocabulary_CheckDDLMatchesGoSet(t *testing.T) {
	start := strings.Index(pageLinksTableDDL, "relation IN (")
	if start < 0 {
		t.Fatal("pageLinksTableDDL lost the relation IN-list")
	}
	list := pageLinksTableDDL[start+len("relation IN ("):]
	list = list[:strings.Index(list, ")")]
	seen := map[string]bool{}
	for _, tok := range strings.Split(list, ",") {
		tok = strings.Trim(strings.TrimSpace(tok), "'")
		if tok == "" {
			continue
		}
		seen[tok] = true
		if !ValidRelation(tok) {
			t.Errorf("CHECK allows %q but ValidRelation rejects it", tok)
		}
	}
	if len(seen) != len(wikiRelations) {
		t.Errorf("CHECK lists %d relations, Go set has %d", len(seen), len(wikiRelations))
	}
}

// v21 must upgrade a PRE-v21 page_links (free-text relation) to the constrained
// one: normalize legacy values, collapse parallel edges that become duplicates,
// keep legal ones, refuse an off-vocabulary insert afterwards, and no-op on replay.
func TestWikiRelationVocabulary_MigratesLegacyTable(t *testing.T) {
	db := setupTestDB(t)
	if _, err := db.Exec(`DROP TABLE page_links`); err != nil {
		t.Fatal(err)
	}
	const oldDDL = `CREATE TABLE page_links (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_page_id INTEGER NOT NULL,
		to_page_id INTEGER NOT NULL,
		relation TEXT NOT NULL DEFAULT 'ref',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		CHECK (from_page_id <> to_page_id),
		UNIQUE (from_page_id, to_page_id, relation),
		FOREIGN KEY (from_page_id) REFERENCES wiki_pages(id) ON DELETE CASCADE,
		FOREIGN KEY (to_page_id) REFERENCES wiki_pages(id) ON DELETE CASCADE
	)`
	if _, err := db.Exec(oldDDL); err != nil {
		t.Fatal(err)
	}
	a, _ := CreateWikiPage(db, WikiKindEntity, "a", "A", 0, "")
	b, _ := CreateWikiPage(db, WikiKindEntity, "b", "B", 0, "")
	c, _ := CreateWikiPage(db, WikiKindEntity, "c", "C", 0, "")
	seed := []struct {
		f, t int64
		rel  string
	}{
		{a.ID, b.ID, "cites"},         // alias → ref
		{a.ID, b.ID, "compiled-from"}, // alias → ref, collides with the row above
		{a.ID, c.ID, "wildcard"},      // unknown → ref
		{b.ID, c.ID, "depends"},       // canonical → kept
	}
	for _, s := range seed {
		if _, err := db.Exec(
			`INSERT INTO page_links(from_page_id,to_page_id,relation) VALUES(?,?,?)`, s.f, s.t, s.rel); err != nil {
			t.Fatal(err)
		}
	}

	if err := EnsureWikiRelationVocabulary(db); err != nil {
		t.Fatalf("v21: %v", err)
	}
	var ddl string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='page_links'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ddl, "relation IN (") {
		t.Fatal("migration did not add the relation CHECK")
	}
	// 4 seeds → 3 after the two a→b edges collapse to one ref.
	if n := countPageLinks(t, db); n != 3 {
		t.Errorf("edges after v21 = %d, want 3 (two a->b collapsed to one ref)", n)
	}
	var rel string
	if err := db.QueryRow(`SELECT relation FROM page_links WHERE from_page_id=? AND to_page_id=?`, b.ID, c.ID).Scan(&rel); err != nil {
		t.Fatal(err)
	}
	if rel != RelationDepends {
		t.Errorf("canonical 'depends' became %q; migration must preserve legal values", rel)
	}
	if _, err := db.Exec(`INSERT INTO page_links(from_page_id,to_page_id,relation) VALUES(?,?,?)`, a.ID, b.ID, "nope"); err == nil {
		t.Error("CHECK not enforced after migration")
	}
	before := countPageLinks(t, db)
	if err := EnsureWikiRelationVocabulary(db); err != nil {
		t.Fatalf("v21 replay: %v", err)
	}
	if after := countPageLinks(t, db); after != before {
		t.Errorf("replay changed edge count %d -> %d", before, after)
	}
}

func countPageLinks(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM page_links`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
