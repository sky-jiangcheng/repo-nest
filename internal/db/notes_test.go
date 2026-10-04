package db

import (
	"strings"
	"testing"
)

// The bounds are enforced at the DB layer because this package is the
// chokepoint every writer converges on — the service layer (desktop UI, MCP
// tools) AND the plugin runtime's import upserts, which call these functions
// directly. These tests prove the second path is bounded too.
func TestNoteWriteBoundsAtDBLayer(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "proj-a")

	// Sanity: normal-size writes must succeed, or the bounds are too tight.
	note, err := CreateNoteEx(db, pid, "title", "body", "tag", "other", "manual")
	if err != nil {
		t.Fatalf("CreateNoteEx: %v", err)
	}

	cases := []struct {
		name string
		fn   func() error
	}{
		{"CreateNoteEx oversized content", func() error {
			_, err := CreateNoteEx(db, pid, "t", strings.Repeat("x", MaxNoteContentLen+1), "", "knowledge", "manual")
			return err
		}},
		{"CreateNoteEx oversized title", func() error {
			_, err := CreateNoteEx(db, pid, strings.Repeat("t", MaxNoteTitleLen+1), "body", "", "knowledge", "manual")
			return err
		}},
		{"CreateNoteEx too many tags", func() error {
			_, err := CreateNoteEx(db, pid, "t", "body", strings.Repeat("a,", MaxNoteTagCount+1), "knowledge", "manual")
			return err
		}},
		{"UpdateNote oversized content", func() error {
			return UpdateNote(db, note.ID, strings.Repeat("x", MaxNoteContentLen+1))
		}},
		{"UpdateNoteFull oversized title", func() error {
			return UpdateNoteFull(db, note.ID, "body", strings.Repeat("t", MaxNoteTitleLen+1), "tag", "knowledge", false)
		}},
		{"UpdateNoteMeta oversized title", func() error {
			return UpdateNoteMeta(db, note.ID, strings.Repeat("t", MaxNoteTitleLen+1), "tag", "knowledge", false)
		}},
		{"UpdateNoteMeta too many tags", func() error {
			return UpdateNoteMeta(db, note.ID, "title", strings.Repeat("a,", MaxNoteTagCount+1), "knowledge", false)
		}},
	}
	for _, c := range cases {
		if err := c.fn(); err == nil {
			t.Errorf("%s: expected rejection", c.name)
		}
	}

	// The note must be untouched by the rejected writes.
	got, err := GetNoteByID(db, note.ID)
	if err != nil {
		t.Fatalf("GetNoteByID: %v", err)
	}
	if got.Title != "title" || got.Content != "body" || got.Tags != "tag" {
		t.Errorf("note mutated by rejected writes: %+v", got)
	}
}

// Deleting a note must not leave its embedding behind: orphaned vectors
// consume KNN's k budget and are invisible to the inner-join materialisation,
// so semantic recall degrades silently until a full rebuild.
func TestDeleteNoteRemovesEmbedding(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	pid := createTestProject(t, database, "vec")
	note, err := CreateNote(database, pid, "to be deleted")
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureVectorIndex(database, 2); err != nil {
		t.Fatal(err)
	}
	if err := PutNoteEmbedding(database, note.ID, []float32{1, 0}); err != nil {
		t.Fatal(err)
	}

	if err := DeleteNote(database, note.ID); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	ids, err := KnnNoteIDs(database, []float32{1, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("embedding for deleted note %d survived: %v", note.ID, ids)
	}

	// Cascade path: prune removes vectors for notes deleted outside DeleteNote.
	other, err := CreateNote(database, pid, "cascade deleted")
	if err != nil {
		t.Fatal(err)
	}
	if err := PutNoteEmbedding(database, other.ID, []float32{0, 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("DELETE FROM project_notes WHERE id = ?", other.ID); err != nil {
		t.Fatal(err)
	}
	n, err := PruneNoteEmbeddings(database)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("prune should remove 1 orphan, removed %d", n)
	}
	ids, _ = KnnNoteIDs(database, []float32{0, 1}, 5)
	if len(ids) != 0 {
		t.Fatalf("orphaned embedding survived prune: %v", ids)
	}
}
