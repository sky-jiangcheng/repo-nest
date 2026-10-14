package db

import (
	"testing"
)

// The page vector index is a derived cache (ADR-0012 决策 5), so the acceptance
// bar is the same one note_embeddings already meets: ensure at a dim, put, KNN
// round-trips the id, delete removes it, and prune drops vectors whose page left
// the approved set. These mirror the note-vec tests so the two indexes cannot
// drift in behaviour.

func TestPageVectorIndex_RoundTrip(t *testing.T) {
	db := setupTestDB(t)
	if PageVectorIndexReady(db) {
		t.Fatal("page index should not exist before it is created")
	}
	if err := EnsurePageVectorIndex(db, 4); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !PageVectorIndexReady(db) {
		t.Fatal("page index missing after ensure")
	}
	// Idempotent at the same dim: no drop/rebuild.
	if err := EnsurePageVectorIndex(db, 4); err != nil {
		t.Fatalf("re-ensure same dim: %v", err)
	}

	vec := []float32{0.1, 0.2, 0.3, 0.4}
	if err := PutPageEmbedding(db, 42, vec); err != nil {
		t.Fatalf("put: %v", err)
	}
	ids, err := KnnPageIDs(db, vec, 5)
	if err != nil {
		t.Fatalf("knn: %v", err)
	}
	if len(ids) != 1 || ids[0] != 42 {
		t.Fatalf("knn ids = %v, want [42]", ids)
	}
	if err := DeletePageEmbedding(db, 42); err != nil {
		t.Fatalf("delete: %v", err)
	}
	ids, err = KnnPageIDs(db, vec, 5)
	if err != nil {
		t.Fatalf("knn after delete: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("knn after delete = %v, want none", ids)
	}
}

// A model/dim swap must rebuild, not silently mis-rank: Ensure drops a table of a
// different dim so the next rebuild repopulates at the new width.
func TestPageVectorIndex_DimChangeRebuilds(t *testing.T) {
	db := setupTestDB(t)
	if err := EnsurePageVectorIndex(db, 4); err != nil {
		t.Fatalf("ensure 4: %v", err)
	}
	if err := PutPageEmbedding(db, 1, []float32{1, 0, 0, 0}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := EnsurePageVectorIndex(db, 8); err != nil {
		t.Fatalf("ensure 8: %v", err)
	}
	if dim, ok := StoredPageVectorDim(db); !ok || dim != 8 {
		t.Fatalf("stored dim = %d,%v want 8", dim, ok)
	}
	// The dim-4 vector is gone (the table was dropped and recreated at width 8).
	ids, err := KnnPageIDs(db, []float32{1, 0, 0, 0, 0, 0, 0, 0}, 5)
	if err != nil {
		t.Fatalf("knn: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("stale dim-4 vector survived the rebuild: %v", ids)
	}
}

// The page index and the note index are PHYSICALLY SEPARATE vec0 tables: the same
// rowid must be addressable in each without collision. This is the structural
// guard behind ADR-0018's "never fuse page ids with note ids in one RRF" rule.
func TestPageAndNoteVectorIndexesAreSeparate(t *testing.T) {
	db := setupTestDB(t)
	if err := EnsureVectorIndex(db, 4); err != nil {
		t.Fatalf("ensure note index: %v", err)
	}
	if err := EnsurePageVectorIndex(db, 4); err != nil {
		t.Fatalf("ensure page index: %v", err)
	}
	vec := []float32{0.5, 0.5, 0.5, 0.5}
	if err := PutNoteEmbedding(db, 7, vec); err != nil {
		t.Fatalf("put note: %v", err)
	}
	if err := PutPageEmbedding(db, 7, vec); err != nil {
		t.Fatalf("put page: %v", err)
	}
	noteIDs, err := KnnNoteIDs(db, vec, 5)
	if err != nil {
		t.Fatalf("note knn: %v", err)
	}
	pageIDs, err := KnnPageIDs(db, vec, 5)
	if err != nil {
		t.Fatalf("page knn: %v", err)
	}
	if len(noteIDs) != 1 || len(pageIDs) != 1 {
		t.Fatalf("note=%v page=%v, want one each", noteIDs, pageIDs)
	}
	// Both legitimately hold rowid 7; they are different objects in different
	// tables, and neither KNN sees the other's row.
	if noteIDs[0] != 7 || pageIDs[0] != 7 {
		t.Fatalf("note=%v page=%v, want [7] each", noteIDs, pageIDs)
	}
}

// Only APPROVED pages are embedded (ADR-0015): ListPageEmbeddingInputs must skip
// pending/rejected pages, because a vector that feeds AskAI evidence must only ever
// come from content a human reviewed.
func TestListPageEmbeddingInputs_OnlyApproved(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "pagevec")
	if _, err := CreateWikiPage(db, WikiKindConcept, "appr", "Approved", pid, "approved body"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateWikiPageAs(db, WikiKindConcept, "pend", "Pending", pid, "pending body", WikiStatusPending, "wiki-compile"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateWikiPageAs(db, WikiKindConcept, "rej", "Rejected", pid, "rejected body", WikiStatusRejected, "wiki-compile"); err != nil {
		t.Fatal(err)
	}
	inputs, err := ListPageEmbeddingInputs(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 {
		t.Fatalf("inputs = %d, want exactly the one approved page", len(inputs))
	}
	if want := PageEmbeddingText("Approved", "approved body"); inputs[0].Text != want {
		t.Errorf("text = %q, want %q (shared builder)", inputs[0].Text, want)
	}
}

// A page whose vector survives after it leaves the approved set (or is deleted)
// keeps being recalled forever — the same correctness bug the note prune guards
// against. PrunePageEmbeddings must drop both cases.
func TestPrunePageEmbeddings_DropsGoneAndUnapproved(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "prune")
	if err := EnsurePageVectorIndex(db, 4); err != nil {
		t.Fatal(err)
	}
	keep, err := CreateWikiPage(db, WikiKindConcept, "keep", "Keep", pid, "keep")
	if err != nil {
		t.Fatal(err)
	}
	unappr, err := CreateWikiPageAs(db, WikiKindConcept, "unappr", "Unapproved", pid, "x", WikiStatusPending, "wiki-compile")
	if err != nil {
		t.Fatal(err)
	}
	vec := []float32{0.25, 0.25, 0.25, 0.25}
	// Seed vectors for a live-approved page and a since-pending page directly
	// (bypassing the drainer, which would not have embedded the pending one).
	if err := PutPageEmbedding(db, keep.ID, vec); err != nil {
		t.Fatal(err)
	}
	if err := PutPageEmbedding(db, unappr.ID, vec); err != nil {
		t.Fatal(err)
	}
	// A vector for a page that no longer exists at all.
	if err := PutPageEmbedding(db, 99999, vec); err != nil {
		t.Fatal(err)
	}
	removed, err := PrunePageEmbeddings(db)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("pruned %d, want 2 (the pending page and the ghost)", removed)
	}
	ids, err := KnnPageIDs(db, vec, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != keep.ID {
		t.Fatalf("surviving ids = %v, want only the approved page %d", ids, keep.ID)
	}
}

func TestPageVectorStoreHealthCheck(t *testing.T) {
	db := setupTestDB(t)
	if err := PageVectorStoreHealthCheck(db, 3); err != nil {
		t.Fatalf("health check: %v", err)
	}
	// The probe vector must be cleaned up, not left as a phantom page vector.
	ids, err := KnnPageIDs(db, []float32{0.5, 0.5, 0.5}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("health check left a probe vector: %v", ids)
	}
}
