package service

import (
	"testing"

	"repotory/internal/db"
)

// The page twin of TestDrainIndexesNewNotesAndUnIndexesDeletedOnes. The claims:
// RebuildPageEmbeddings fills the SEPARATE page_embeddings index; a page edited or
// newly approved afterwards is picked up by the drainer without another full
// rebuild; and un-approving or deleting a page removes its vector (a stale vector
// for a since-pending page would keep it in recall forever).
func TestPageDrainIndexesAndUnIndexesPages(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "pdrain", "/tmp/pdrain")
	srv, hits := countedEmbedServer(t, false)
	configureEmbedding(t, svc, srv.URL, "2")

	page, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "p1", "Page One", pid, "body")
	if err != nil {
		t.Fatal(err)
	}
	// First rebuild creates the page index (learning dim from notes' persisted
	// embedding_dim) and clears the queue.
	n, err := svc.RebuildPageEmbeddings()
	if err != nil || n != 1 {
		t.Fatalf("rebuild pages: n=%d err=%v", n, err)
	}
	if !db.PageVectorIndexReady(svc.db) {
		t.Fatal("page index not created by RebuildPageEmbeddings")
	}

	// A new approved page (direct db write = the compiler/approve path) queues work.
	page2, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "p2", "Page Two", pid, "more")
	if err != nil {
		t.Fatal(err)
	}
	before := hits.Load()
	got, err := svc.drainDirtyPages(10)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if got != 1 {
		t.Fatalf("drain resolved %d, want 1 (the new page)", got)
	}
	if hits.Load() <= before {
		t.Fatal("drain resolved the page without calling the endpoint")
	}
	ids, err := db.KnnPageIDs(svc.db, []float32{1, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("page KNN after drain = %v, want both pages", ids)
	}

	// Un-approving page 1 must drop its vector on the next drain (no endpoint call).
	if err := db.SetWikiPageStatus(svc.db, page.ID, db.WikiStatusPending); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.drainDirtyPages(10); err != nil || got != 1 {
		t.Fatalf("drain after un-approve: got=%d err=%v, want 1 resolved", got, err)
	}
	ids, err = db.KnnPageIDs(svc.db, []float32{1, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if id == page.ID {
			t.Fatalf("un-approved page %d still in the page index: %v", page.ID, ids)
		}
	}

	// Deleting page 2 must also remove its vector.
	if err := db.DeleteWikiPage(svc.db, page2.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.drainDirtyPages(10); err != nil || got != 1 {
		t.Fatalf("drain after delete: got=%d err=%v, want 1 resolved", got, err)
	}
	ids, err = db.KnnPageIDs(svc.db, []float32{1, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("page index not empty after all pages un-approved/deleted: %v", ids)
	}
}

// Only APPROVED pages are embedded: RebuildPageEmbeddings must skip pending pages,
// and a pending page created before the first rebuild must never reach the index
// (its content is unreviewed model output, ADR-0015).
func TestRebuildPageEmbeddings_OnlyApproved(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "pappr", "/tmp/pappr")
	srv, _ := countedEmbedServer(t, false)
	configureEmbedding(t, svc, srv.URL, "2")

	if _, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "ok", "OK", pid, "good"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateWikiPageAs(svc.db, db.WikiKindConcept, "pend", "Pend", pid, "draft", db.WikiStatusPending, "wiki-compile"); err != nil {
		t.Fatal(err)
	}
	n, err := svc.RebuildPageEmbeddings()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("embedded %d pages, want 1 (only the approved one)", n)
	}
}

// semantic_search is the consent to send text out; without it, no endpoint call
// may ever happen for pages. This is the same gate the note path uses, and the
// reason page embedding introduces no second switch.
func TestPageEmbeddings_GatedBySemanticSearch(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "pgate", "/tmp/pgate")
	// Configure the endpoint but leave semantic_search OFF.
	srv, hits := countedEmbedServer(t, false)
	configureEmbedding(t, svc, srv.URL, "2")
	if err := svc.UpdateConfig("semantic_search", "0"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "p", "P", pid, "body"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RebuildPageEmbeddings(); err == nil {
		t.Fatal("RebuildPageEmbeddings succeeded with semantic_search off; it must refuse")
	}
	if got, err := svc.drainDirtyPages(10); err != nil || got != 0 {
		t.Fatalf("drain with semantic off: got=%d err=%v, want 0/no-op", got, err)
	}
	if hits.Load() != 0 {
		t.Fatalf("endpoint was called %d time(s) with semantic_search off", hits.Load())
	}
}

// A drain whose endpoint dies must leave the queued rows for retry — the durable
// queue, not the return value, is what keeps a failed round from losing pages.
func TestPageDrainRetainsQueueOnEndpointFailure(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "pfail", "/tmp/pfail")
	good, _ := countedEmbedServer(t, false)
	configureEmbedding(t, svc, good.URL, "2")
	// Build the index once so the drainer has somewhere to write.
	if _, err := svc.RebuildPageEmbeddings(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "p", "P", pid, "body"); err != nil {
		t.Fatal(err)
	}
	dead, _ := countedEmbedServer(t, true)
	if err := svc.UpdateConfig("embedding_base_url", dead.URL+"/v1"); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.drainDirtyPages(10); err != nil || got != 0 {
		t.Fatalf("drain with dead endpoint: got=%d err=%v, want 0 resolved (queued for retry)", got, err)
	}
	var queued int
	if err := svc.db.QueryRow("SELECT COUNT(*) FROM page_embed_dirty").Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued == 0 {
		t.Fatal("failed drain consumed the queue; the page would be lost until a rebuild")
	}
}

// The page index is physically separate from the note index: embedding a page must
// never land in note_embeddings and vice versa. This is the runtime expression of
// ADR-0018's "never fuse page ids with note ids in one RRF" rule.
func TestPageAndNoteIndexesStaySeparate(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "psep", "/tmp/psep")
	srv, _ := countedEmbedServer(t, false)
	configureEmbedding(t, svc, srv.URL, "2")

	if _, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "p", "P", pid, "page body"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateNoteEx(svc.db, pid, "n", "note body", "", "other", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RebuildEmbeddings(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RebuildPageEmbeddings(); err != nil {
		t.Fatal(err)
	}
	noteIDs, err := db.KnnNoteIDs(svc.db, []float32{1, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}
	pageIDs, err := db.KnnPageIDs(svc.db, []float32{1, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(noteIDs) != 1 || len(pageIDs) != 1 {
		t.Fatalf("note=%v page=%v, want one each in their own index", noteIDs, pageIDs)
	}
}
