package db

import (
	"database/sql"
	"strings"
	"testing"
)

// setupMergeDB creates two projects and two notes inside the first, so the
// merge tests can attach real provenance (note_pages has FKs on both ends —
// bare ids would trip the constraint) and exercise the cross-project refusal.
func setupMergeDB(t *testing.T) (database *sql.DB, projectA, projectB int64) {
	t.Helper()
	database = setupTestDB(t)
	t.Cleanup(func() { _ = database.Close() })
	projectA = createTestProject(t, database, "merge-a")
	projectB = createTestProject(t, database, "merge-b")
	for _, title := range []string{"source note 1", "source note 2"} {
		if _, err := CreateNoteEx(database, projectA, title, "note body", "", "other", "manual"); err != nil {
			t.Fatalf("CreateNoteEx %q: %v", title, err)
		}
	}
	return database, projectA, projectB
}

// ADR-0019 晋升条件 2: the merge contract, pinned piece by piece.
//   (a) winner's content/updated_at are not touched by the merge;
//   (b) afterwards the loser has no edges and is a rejected tombstone;
//   (c) note_pages provenance moves to the winner, nothing dropped;
//   (d) replaying the same pair is idempotent.
func TestMergeWikiPages_Contract(t *testing.T) {
	db, projectA, _ := setupMergeDB(t)

	winner, err := CreateWikiPageAs(db, "concept", "payment-gateway", "Payment Gateway", projectA, "canonical body", WikiStatusApproved, "manual")
	if err != nil {
		t.Fatalf("CreateWikiPageAs winner: %v", err)
	}
	loser, err := CreateWikiPageAs(db, "concept", "gateway-payment", "Gateway Payment", projectA, "duplicate body", WikiStatusPending, "wiki-compile")
	if err != nil {
		t.Fatalf("CreateWikiPageAs loser: %v", err)
	}
	other, err := CreateWikiPage(db, "concept", "retry-policy", "Retry Policy", projectA, "retries")
	if err != nil {
		t.Fatalf("CreateWikiPage other: %v", err)
	}

	// Edges: loser→other (out), other→loser (in), loser→winner (would become a
	// self-loop), other→winner already exists (must be absorbed, not duplicated).
	if err := LinkWikiPages(db, loser.ID, other.ID, RelationRef); err != nil {
		t.Fatalf("link loser→other: %v", err)
	}
	if err := LinkWikiPages(db, other.ID, loser.ID, RelationMentions); err != nil {
		t.Fatalf("link other→loser: %v", err)
	}
	if err := LinkWikiPages(db, loser.ID, winner.ID, RelationRef); err != nil {
		t.Fatalf("link loser→winner: %v", err)
	}
	if err := LinkWikiPages(db, other.ID, winner.ID, RelationRef); err != nil {
		t.Fatalf("link other→winner: %v", err)
	}
	// Provenance: two source notes on the loser, one already on the winner.
	notes, err := ListNotes(db, projectA)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("setup should create 2 notes, got %d", len(notes))
	}
	note1, note2 := notes[0].ID, notes[1].ID
	if err := AttachNoteToPage(db, note1, loser.ID); err != nil {
		t.Fatalf("attach note 1: %v", err)
	}
	if err := AttachNoteToPage(db, note2, loser.ID); err != nil {
		t.Fatalf("attach note 2: %v", err)
	}
	if err := AttachNoteToPage(db, note2, winner.ID); err != nil {
		t.Fatalf("attach note 2 to winner: %v", err)
	}

	winnerBefore, err := GetWikiPageByID(db, winner.ID)
	if err != nil {
		t.Fatalf("GetWikiPageByID before: %v", err)
	}

	if err := MergeWikiPages(db, winner.ID, loser.ID); err != nil {
		t.Fatalf("MergeWikiPages: %v", err)
	}

	// (a) winner untouched: every field, including the wall clock.
	winnerAfter, err := GetWikiPageByID(db, winner.ID)
	if err != nil {
		t.Fatalf("GetWikiPageByID after: %v", err)
	}
	if *winnerBefore != *winnerAfter {
		t.Fatalf("winner changed by the merge:\n before=%+v\n after =%+v", *winnerBefore, *winnerAfter)
	}

	// (b) loser has no edges left and is a rejected tombstone pointing at the
	// winner's slug.
	out, err := WikiEdgesFrom(db, loser.ID)
	if err != nil {
		t.Fatalf("WikiEdgesFrom loser: %v", err)
	}
	in, err := WikiEdgesTo(db, loser.ID)
	if err != nil {
		t.Fatalf("WikiEdgesTo loser: %v", err)
	}
	if len(out) != 0 || len(in) != 0 {
		t.Fatalf("loser still has edges: out=%+v in=%+v", out, in)
	}
	loserAfter, err := GetWikiPageByID(db, loser.ID)
	if err != nil {
		t.Fatalf("GetWikiPageByID loser: %v", err)
	}
	if loserAfter.Status != WikiStatusRejected {
		t.Fatalf("loser status = %q, want rejected", loserAfter.Status)
	}
	if !strings.Contains(loserAfter.Content, "已并入") || !strings.Contains(loserAfter.Content, winner.Slug) {
		t.Fatalf("loser tombstone should point at the winner, got %q", loserAfter.Content)
	}

	// The winner now carries the re-pointed edges: other→winner (ref) absorbed
	// the pre-existing one (no duplicate), winner→other new, other→winner
	// mentions new. The loser→winner self-loop was dropped.
	winnerOut, err := WikiEdgesFrom(db, winner.ID)
	if err != nil {
		t.Fatalf("WikiEdgesFrom winner: %v", err)
	}
	winnerIn, err := WikiEdgesTo(db, winner.ID)
	if err != nil {
		t.Fatalf("WikiEdgesTo winner: %v", err)
	}
	if len(winnerOut) != 1 || winnerOut[0].PageID != other.ID || winnerOut[0].Relation != RelationRef {
		t.Fatalf("winner outbound edges = %+v, want [other ref]", winnerOut)
	}
	if len(winnerIn) != 2 {
		t.Fatalf("winner inbound edges = %+v, want 2 (ref + mentions)", winnerIn)
	}
	var edgeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM page_links").Scan(&edgeCount); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if edgeCount != 3 {
		t.Fatalf("total edges = %d, want 3 (self-loop dropped, duplicate absorbed)", edgeCount)
	}

	// (c) provenance moved: the winner holds notes 1 and 2, the loser none.
	winnerNotes, err := NotesForPage(db, winner.ID)
	if err != nil {
		t.Fatalf("NotesForPage winner: %v", err)
	}
	loserNotes, err := NotesForPage(db, loser.ID)
	if err != nil {
		t.Fatalf("NotesForPage loser: %v", err)
	}
	if len(winnerNotes) != 2 || len(loserNotes) != 0 {
		t.Fatalf("provenance not moved: winner=%v loser=%v", winnerNotes, loserNotes)
	}

	// (d) replay: no error, no new edges, no new attachments, tombstone stable.
	if err := MergeWikiPages(db, winner.ID, loser.ID); err != nil {
		t.Fatalf("MergeWikiPages replay: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM page_links").Scan(&edgeCount); err != nil {
		t.Fatalf("count edges after replay: %v", err)
	}
	if edgeCount != 3 {
		t.Fatalf("replay changed the edge count: %d", edgeCount)
	}
	winnerNotes2, err := NotesForPage(db, winner.ID)
	if err != nil {
		t.Fatalf("NotesForPage winner after replay: %v", err)
	}
	if len(winnerNotes2) != 2 {
		t.Fatalf("replay changed provenance: %v", winnerNotes2)
	}
}

// ADR-0019 待决 4: no cross-project merges in the MVP (a global page, id 0,
// may still merge into either side).
func TestMergeWikiPages_RefusesCrossProject(t *testing.T) {
	db, projectA, projectB := setupMergeDB(t)

	inA, err := CreateWikiPage(db, "concept", "page-a", "Page A", projectA, "a")
	if err != nil {
		t.Fatalf("CreateWikiPage a: %v", err)
	}
	inB, err := CreateWikiPage(db, "concept", "page-b", "Page B", projectB, "b")
	if err != nil {
		t.Fatalf("CreateWikiPage b: %v", err)
	}
	global, err := CreateWikiPage(db, "concept", "page-global", "Global", 0, "g")
	if err != nil {
		t.Fatalf("CreateWikiPage global: %v", err)
	}
	global2, err := CreateWikiPage(db, "concept", "page-global-2", "Global 2", 0, "g2")
	if err != nil {
		t.Fatalf("CreateWikiPage global 2: %v", err)
	}

	if err := MergeWikiPages(db, inA.ID, inB.ID); err == nil {
		t.Fatal("cross-project merge must be refused")
	}
	// A global page may merge into a project page and vice versa — each
	// direction gets its own global page, since a merge tombstones the loser
	// and a rejected page can no longer be a winner.
	if err := MergeWikiPages(db, inA.ID, global.ID); err != nil {
		t.Fatalf("global loser into project winner must be allowed: %v", err)
	}
	if err := MergeWikiPages(db, global2.ID, inB.ID); err != nil {
		t.Fatalf("project loser into global winner must be allowed: %v", err)
	}
}

// Structural refusals: self-merge, and merging INTO a rejected tombstone
// (there is nothing to absorb into).
func TestMergeWikiPages_RefusesSelfAndRejectedWinner(t *testing.T) {
	db, projectA, _ := setupMergeDB(t)

	page, err := CreateWikiPage(db, "concept", "self-page", "Self", projectA, "body")
	if err != nil {
		t.Fatalf("CreateWikiPage: %v", err)
	}
	if err := MergeWikiPages(db, page.ID, page.ID); err == nil {
		t.Fatal("self-merge must be refused")
	}
	if err := SetWikiPageStatus(db, page.ID, WikiStatusRejected); err != nil {
		t.Fatalf("SetWikiPageStatus: %v", err)
	}
	other, err := CreateWikiPage(db, "concept", "other-page", "Other", projectA, "other")
	if err != nil {
		t.Fatalf("CreateWikiPage other: %v", err)
	}
	if err := MergeWikiPages(db, page.ID, other.ID); err == nil {
		t.Fatal("merging into a rejected page must be refused")
	}
}

// The tombstone is a real content change, so the v23 trigger records it as
// the loser's newest version — and the merge's explicit pre-image keeps the
// displaced body as the entry below it (the trigger snapshots the NEW state,
// the v8/v11 convention; without the pre-image a merge would silently destroy
// the loser's content). History only grows.
func TestMergeWikiPages_TombstoneSnapshotted(t *testing.T) {
	db, projectA, _ := setupMergeDB(t)

	winner, err := CreateWikiPage(db, "concept", "merge-winner", "Winner", projectA, "canonical")
	if err != nil {
		t.Fatalf("CreateWikiPage winner: %v", err)
	}
	loser, err := CreateWikiPage(db, "concept", "merge-loser", "Loser", projectA, "duplicate body")
	if err != nil {
		t.Fatalf("CreateWikiPage loser: %v", err)
	}
	if err := MergeWikiPages(db, winner.ID, loser.ID); err != nil {
		t.Fatalf("MergeWikiPages: %v", err)
	}
	versions, err := ListPageVersions(db, loser.ID)
	if err != nil {
		t.Fatalf("ListPageVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("loser history should hold pre-image + tombstone, got %+v", versions)
	}
	// Newest first (created_at DESC, id DESC tiebreak): the tombstone state,
	// then the displaced pre-merge body.
	if !strings.Contains(versions[0].Content, "已并入") {
		t.Fatalf("newest version should be the tombstone, got %q", versions[0].Content)
	}
	if versions[1].Content != "duplicate body" {
		t.Fatalf("the displaced body should survive as the loser's previous version, got %q", versions[1].Content)
	}
}
