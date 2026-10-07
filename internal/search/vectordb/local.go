package vectordb

import (
	"database/sql"

	"repo-nest/internal/db"
)

// Local is the default store: sqlite-vec `note_embeddings` living inside the
// app's dashboard.db (wraps the internal/db vec helpers, ADR-0013).
type Local struct{ database *sql.DB }

// NewLocal binds the sqlite-vec store to an opened database.
func NewLocal(database *sql.DB) *Local { return &Local{database: database} }

func (l *Local) Name() string { return "local-sqlite-vec" }

func (l *Local) Ensure(dim int) error { return db.EnsureVectorIndex(l.database, dim) }

func (l *Local) Clear(_ int) error { return db.ClearVectorIndex(l.database) }

func (l *Local) Upsert(id int64, vec []float32) error {
	return db.PutNoteEmbedding(l.database, id, vec)
}

// Delete drops one note's vector. sqlite-vec has no row for a note that was
// never embedded, so the DELETE is inherently idempotent.
func (l *Local) Delete(id int64) error {
	return db.DeleteNoteEmbedding(l.database, id)
}

func (l *Local) Search(vec []float32, limit int) ([]int64, error) {
	return db.KnnNoteIDs(l.database, vec, limit)
}

// Ready reports whether the derived vec0 index exists at all. Writing into a
// never-created index only errors, so incremental indexing asks this first and
// leaves its queue untouched until a rebuild creates it.
func (l *Local) Ready() bool { return db.VectorIndexReady(l.database) }
