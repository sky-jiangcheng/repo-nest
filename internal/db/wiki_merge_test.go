package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// ADR-0019 晋升条件 2 — the merge transaction contract, asserted at the layer that
// owns it. These four properties are the difference between "a merge tool" and
// "a way to lose knowledge with extra steps", so each gets its own test rather
// than one big scenario where a failure could hide behind an unrelated assert.

func edgePairKey(from, to int64, relation string) string {
	return fmt.Sprintf("%d->%d:%s", from, to, relation)
}

// insertTestNote files a raw note and returns its id. Notes are the provenance
// leaves the wiki layer hangs pages off, so merge tests need real rows rather
// than a stub: note_pages has a foreign key, and it is meant to.
func insertTestNote(t *testing.T, database *sql.DB, projectID int64, content string) int64 {
	t.Helper()
	res, err := database.Exec(
		`INSERT INTO project_notes(project_id, title, content, kind, source) VALUES (?, '', ?, 'other', 'manual')`,
		projectID, content)
	if err != nil {
		t.Fatalf("insert note: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("note id: %v", err)
	}
	return id
}

func liveEdges(t *testing.T, database *sql.DB, pageID int64) []string {
	t.Helper()
	rows, err := database.Query(
		`SELECT from_page_id, to_page_id, relation FROM page_links
		  WHERE from_page_id = ? OR to_page_id = ? ORDER BY relation, from_page_id, to_page_id`,
		pageID, pageID)
	if err != nil {
		t.Fatalf("read edges: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var from, to int64
		var rel string
		if err := rows.Scan(&from, &to, &rel); err != nil {
			t.Fatalf("scan edge: %v", err)
		}
		out = append(out, edgePairKey(from, to, rel))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func pageRow(t *testing.T, database *sql.DB, id int64) *WikiPage {
	t.Helper()
	p, err := GetWikiPageByID(database, id)
	if err != nil {
		t.Fatalf("load page %d: %v", id, err)
	}
	return p
}

// 晋升条件 2a: the winner is byte-for-byte untouched, including updated_at. This is
// what keeps ADR-0015 决策 3 ("nothing may modify an approved page") true in a
// world where a merge tool exists.
func TestMergeWikiPages_WinnerIsNeverTouched(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "merge-winner")

	winner, err := CreateWikiPage(database, WikiKindEntity, "payment-gateway", "Payment Gateway", proj,
		"The gateway service. Body must survive verbatim.")
	if err != nil {
		t.Fatalf("create winner: %v", err)
	}
	loser, err := CreateWikiPageAs(database, WikiKindConcept, "gateway-payment", "Gateway Payment", proj,
		"Duplicate concept page.", WikiStatusPending, "wiki-compile")
	if err != nil {
		t.Fatalf("create loser: %v", err)
	}

	before := pageRow(t, database, winner.ID)
	if _, err := MergeWikiPages(database, loser.ID, winner.ID, "已并入 [[payment-gateway]]"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	after := pageRow(t, database, winner.ID)

	if before.Content != after.Content || before.Title != after.Title || before.Kind != after.Kind ||
		before.Slug != after.Slug || before.Status != after.Status || before.UpdatedAt != after.UpdatedAt {
		t.Fatalf("the winner row changed at all:\n before=%+v\n after =%+v", before, after)
	}
}

// 晋升条件 2b: after the merge the loser has no edges left and is a rejected
// tombstone, and every edge it had now belongs to the winner — including the ones
// that were duplicates of what the winner already had.
func TestMergeWikiPages_RedirectsEveryEdgeAndTombstonesLoser(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "merge-edges")

	winner, err := CreateWikiPage(database, WikiKindEntity, "w", "Winner", proj, "winner body")
	if err != nil {
		t.Fatalf("create winner: %v", err)
	}
	loser, err := CreateWikiPage(database, WikiKindConcept, "l", "Loser", proj, "loser body")
	if err != nil {
		t.Fatalf("create loser: %v", err)
	}
	citer, err := CreateWikiPage(database, WikiKindSource, "c", "Citer", proj, "citer body")
	if err != nil {
		t.Fatalf("create citer: %v", err)
	}
	other, err := CreateWikiPage(database, WikiKindConcept, "o", "Other", proj, "other body")
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	fresh, err := CreateWikiPage(database, WikiKindConcept, "f", "Fresh", proj, "fresh body")
	if err != nil {
		t.Fatalf("create fresh: %v", err)
	}

	link := func(from, to *WikiPage, rel string) {
		t.Helper()
		if err := LinkWikiPages(database, from.ID, to.ID, rel); err != nil {
			t.Fatalf("link: %v", err)
		}
	}
	// Each of the three fates an edge can meet during an absorption gets its own
	// fixture, so the counters below mean one thing each:
	link(loser, fresh, RelationRef)       // redirected: the winner had nothing like it
	link(citer, loser, RelationDocuments) // redirected, incoming direction
	link(loser, winner, RelationMentions) // dropped: rewriting it would be a self-loop
	link(loser, other, RelationRef)       // dropped: the winner already has this exact edge
	link(winner, other, RelationRef)      // ...which is the row that absorbs it
	link(loser, other, RelationRef)       // idempotent repeat of the previous edge

	res, err := MergeWikiPages(database, loser.ID, winner.ID, "已并入 [[w]]")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if res.EdgesRedirected != 2 {
		t.Fatalf("expected 2 edges redirected (loser->fresh, citer->loser), got %d", res.EdgesRedirected)
	}
	if res.EdgesDropped != 2 {
		t.Fatalf("expected 2 edges dropped (self-loop + duplicate), got %d", res.EdgesDropped)
	}

	if left := liveEdges(t, database, loser.ID); len(left) != 0 {
		t.Fatalf("the loser must hold no edges after a merge, still has %v", left)
	}
	got := map[string]bool{}
	for _, e := range liveEdges(t, database, winner.ID) {
		got[e] = true
	}
	for _, want := range []string{
		edgePairKey(winner.ID, fresh.ID, RelationRef),
		edgePairKey(winner.ID, other.ID, RelationRef),
		edgePairKey(citer.ID, winner.ID, RelationDocuments),
	} {
		if !got[want] {
			t.Fatalf("winner is missing the edge %s, has %v", want, got)
		}
	}
	// The self-loop must not exist anywhere — page_links' CHECK would have refused
	// it, and a silently-swallowed error is exactly what this assertion is for.
	if got[edgePairKey(winner.ID, winner.ID, RelationMentions)] {
		t.Fatal("a winner->winner edge exists; the self-loop should have been dropped")
	}
	// And the collapsed duplicate must have stayed ONE edge: ADR-0019 决策 5 leans
	// on "winner 已有同边则被 UNIQUE 丢弃", which is only true if the merge does not
	// manufacture a parallel edge under a different id.
	var dupes int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM page_links WHERE from_page_id = ? AND to_page_id = ? AND relation = ?`,
		winner.ID, other.ID, RelationRef).Scan(&dupes); err != nil {
		t.Fatalf("count collapsed edge: %v", err)
	}
	if dupes != 1 {
		t.Fatalf("the collapsed duplicate must remain exactly one edge, found %d", dupes)
	}

	tomb := pageRow(t, database, loser.ID)
	if tomb.Status != WikiStatusRejected {
		t.Fatalf("loser must be rejected after absorption, got %q", tomb.Status)
	}
	if tomb.Content != "已并入 [[w]]" {
		t.Fatalf("loser body must be the tombstone, got %q", tomb.Content)
	}
	if tomb.Slug != "l" {
		t.Fatalf("the loser keeps its slug (reject≠delete), got %q", tomb.Slug)
	}
}

// 晋升条件 2c: provenance moves with the edges, including the ordinary case where
// the winner already had the same note attached.
func TestMergeWikiPages_MovesNoteProvenance(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "merge-notes")

	winner, err := CreateWikiPage(database, WikiKindEntity, "wn", "W", proj, "w")
	if err != nil {
		t.Fatalf("create winner: %v", err)
	}
	loser, err := CreateWikiPage(database, WikiKindConcept, "ln", "L", proj, "l")
	if err != nil {
		t.Fatalf("create loser: %v", err)
	}
	shared := insertTestNote(t, database, proj, "shared source note")
	onlyLoser := insertTestNote(t, database, proj, "note only the loser was built from")

	for _, pair := range []struct {
		note int64
		page *WikiPage
	}{
		{shared, winner}, {shared, loser}, {onlyLoser, loser},
	} {
		if err := AttachNoteToPage(database, pair.note, pair.page.ID); err != nil {
			t.Fatalf("attach: %v", err)
		}
	}

	res, err := MergeWikiPages(database, loser.ID, winner.ID, "已并入 [[wn]]")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if res.NotesReattached != 2 {
		t.Fatalf("both notes should be reattached (one collapsed under the PK), got %d", res.NotesReattached)
	}
	ids, err := NotesForPage(database, winner.ID)
	if err != nil {
		t.Fatalf("notes for winner: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("winner must carry both source notes, got %v", ids)
	}
	loserNotes, err := NotesForPage(database, loser.ID)
	if err != nil {
		t.Fatalf("notes for loser: %v", err)
	}
	if len(loserNotes) != 0 {
		t.Fatalf("the loser must retain no attachments, got %v", loserNotes)
	}
}

// 晋升条件 2d: replaying the same merge is a no-op rather than a second migration.
// The UI can be double-clicked and a job can be retried; neither may corrupt the
// graph, so this is behaviour, not cosmetics.
func TestMergeWikiPages_ReplayIsIdempotent(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "merge-replay")

	winner, err := CreateWikiPage(database, WikiKindEntity, "rw", "W", proj, "w body")
	if err != nil {
		t.Fatalf("create winner: %v", err)
	}
	loser, err := CreateWikiPage(database, WikiKindConcept, "rl", "L", proj, "l body")
	if err != nil {
		t.Fatalf("create loser: %v", err)
	}
	other, err := CreateWikiPage(database, WikiKindSource, "ro", "O", proj, "o body")
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	if err := LinkWikiPages(database, loser.ID, other.ID, RelationRef); err != nil {
		t.Fatalf("link: %v", err)
	}
	note := insertTestNote(t, database, proj, "src")
	if err := AttachNoteToPage(database, note, loser.ID); err != nil {
		t.Fatalf("attach: %v", err)
	}

	first, err := MergeWikiPages(database, loser.ID, winner.ID, "已并入 [[rw]]")
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	if first.EdgesRedirected != 1 || first.NotesReattached != 1 {
		t.Fatalf("first pass should move real work, got %+v", first)
	}
	beforeEdges := len(liveEdges(t, database, winner.ID))

	second, err := MergeWikiPages(database, loser.ID, winner.ID, "已并入 [[rw]]")
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if second.EdgesRedirected != 0 || second.EdgesDropped != 0 || second.NotesReattached != 0 {
		t.Fatalf("a replay must move nothing, got %+v", second)
	}
	if after := len(liveEdges(t, database, winner.ID)); after != beforeEdges {
		t.Fatalf("the replay changed the edge set: %d -> %d", beforeEdges, after)
	}
}

func TestMergeWikiPages_Refusals(t *testing.T) {
	database := setupTestDB(t)
	a := createTestProject(t, database, "proj-a")
	b := createTestProject(t, database, "proj-b")

	inA, err := CreateWikiPage(database, WikiKindEntity, "in-a", "A page", a, "a")
	if err != nil {
		t.Fatalf("create in-a: %v", err)
	}
	inB, err := CreateWikiPage(database, WikiKindConcept, "in-b", "B page", b, "b")
	if err != nil {
		t.Fatalf("create in-b: %v", err)
	}
	global, err := CreateWikiPage(database, WikiKindConcept, "glob", "Global page", 0, "g")
	if err != nil {
		t.Fatalf("create global: %v", err)
	}
	dead, err := CreateWikiPageAs(database, WikiKindConcept, "dead", "Rejected", a,
		"tombstone", WikiStatusRejected, "manual")
	if err != nil {
		t.Fatalf("create dead: %v", err)
	}

	if _, err := MergeWikiPages(database, inA.ID, inA.ID, "x"); !errors.Is(err, ErrMergeSelfLink) {
		t.Fatalf("self-merge must be refused, got %v", err)
	}
	if _, err := MergeWikiPages(database, inA.ID, inB.ID, "x"); !errors.Is(err, ErrMergeCrossProject) {
		t.Fatalf("cross-project merge must be refused (待决 4), got %v", err)
	}
	if _, err := MergeWikiPages(database, inA.ID, dead.ID, "x"); !errors.Is(err, ErrMergeWinnerDead) {
		t.Fatalf("merging into a rejected page must be refused, got %v", err)
	}
	if _, err := MergeWikiPages(database, inA.ID, inB.ID, "  "); !errors.Is(err, ErrMergeNothing) {
		t.Fatalf("an empty tombstone must be refused, got %v", err)
	}
	// A global page may take either side of the merge: that is the exception 待决 4
	// allows, and without this assertion the guard reads as a blanket ban.
	if _, err := MergeWikiPages(database, inA.ID, global.ID, "已并入 [[glob]]"); err != nil {
		t.Fatalf("project page may merge into a global page: %v", err)
	}
	if _, err := MergeWikiPages(database, global.ID, inB.ID, "已并入 [[in-b]]"); err != nil {
		t.Fatalf("global page may merge into a project page: %v", err)
	}
	// Missing pages surface as ErrNoRows rather than merging into nothing.
	if _, err := MergeWikiPages(database, 987654, inB.ID, "x"); err == nil ||
		!strings.Contains(err.Error(), "no rows") {
		t.Fatalf("merging a nonexistent loser must fail loudly, got %v", err)
	}
}

// The interaction between ADR-0019 and ADR-0021 that neither file could verify
// alone: absorbing an APPROVED loser must not be possible to do quietly. The
// v22 snapshot the loser got when it was approved stays (the tombstone write is a
// status change away from 'approved', so no junk version is filed), which is what
// "we can still read what it said before it was absorbed" rests on.
func TestMergeWikiPages_ApprovedLoserKeepsItsSnapshot(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "merge-hist")

	winner, err := CreateWikiPage(database, WikiKindEntity, "hw", "W", proj, "winner")
	if err != nil {
		t.Fatalf("create winner: %v", err)
	}
	loser, err := CreateWikiPageAs(database, WikiKindConcept, "hl", "L", proj,
		"the approved conclusion being absorbed", WikiStatusPending, "wiki-compile")
	if err != nil {
		t.Fatalf("create loser: %v", err)
	}
	if err := SetWikiPageStatus(database, loser.ID, WikiStatusApproved); err != nil {
		t.Fatalf("approve loser: %v", err)
	}
	before, err := ListPageVersions(database, loser.ID)
	if err != nil || len(before) != 1 {
		t.Fatalf("the approved loser should have exactly one snapshot, got %d (%v)", len(before), err)
	}

	if _, err := MergeWikiPages(database, loser.ID, winner.ID, "已并入 [[hw]]"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	after, err := ListPageVersions(database, loser.ID)
	if err != nil {
		t.Fatalf("list after merge: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("filing a tombstone must not add a version, got %d", len(after))
	}
	if after[0].Content != "the approved conclusion being absorbed" {
		t.Fatalf("the pre-merge content must remain readable, got %q", after[0].Content)
	}
}

// NoteOverlapPairs is the one candidate signal that comes from SQL rather than
// from scanning the page list, and it is also where the same-project-or-global
// rule is baked in.
func TestNoteOverlapPairs(t *testing.T) {
	database := setupTestDB(t)
	a := createTestProject(t, database, "ov-a")
	b := createTestProject(t, database, "ov-b")

	left, err := CreateWikiPage(database, WikiKindEntity, "ov-l", "L", a, "l")
	if err != nil {
		t.Fatalf("create left: %v", err)
	}
	right, err := CreateWikiPage(database, WikiKindConcept, "ov-r", "R", a, "r")
	if err != nil {
		t.Fatalf("create right: %v", err)
	}
	elsewhere, err := CreateWikiPage(database, WikiKindConcept, "ov-e", "E", b, "e")
	if err != nil {
		t.Fatalf("create elsewhere: %v", err)
	}
	n1 := insertTestNote(t, database, a, "note 1")
	n2 := insertTestNote(t, database, a, "note 2")

	for _, p := range []struct {
		note int64
		page int64
	}{
		{n1, left.ID}, {n1, right.ID}, {n2, left.ID}, {n2, right.ID},
		// A cross-project pair must never be suggested, so it must not even come
		// back from the blocking query.
		{n1, elsewhere.ID},
	} {
		if err := AttachNoteToPage(database, p.note, p.page); err != nil {
			t.Fatalf("attach: %v", err)
		}
	}

	pairs, err := NoteOverlapPairs(database, a)
	if err != nil {
		t.Fatalf("pairs: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected exactly the (left,right) pair, got %+v", pairs)
	}
	if pairs[0].Left != left.ID || pairs[0].Right != right.ID {
		t.Fatalf("unexpected pair ids: %+v", pairs[0])
	}
	if pairs[0].Count != 2 {
		t.Fatalf("both notes are shared, expected count 2, got %d", pairs[0].Count)
	}
	// projectID 0 is "everything in scope", which must still respect the rule.
	all, err := NoteOverlapPairs(database, 0)
	if err != nil {
		t.Fatalf("all pairs: %v", err)
	}
	for _, p := range all {
		if p.Left == elsewhere.ID || p.Right == elsewhere.ID {
			t.Fatalf("a cross-project pair leaked through: %+v", p)
		}
	}
}
