package vectordb

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/philippgille/chromem-go"
)

// Chromem is a local, embedded, pure-Go vector store (chromem-go) — the third
// backend (ADR-0013 candidate matrix: "本地·纯 Go 库…向量入内存，适合小数据").
// Unlike Local (sqlite-vec) it lives OUTSIDE dashboard.db in its own
// directory, so it neither shares the notes transaction nor the backup; its
// niche is a user who wants a second isolated store (e.g. comparing two
// embedding models side by side) without touching the primary index.
//
// Mapped onto the Store contract:
//   - note id (int64) ↔ chromem document ID (string, decimal, "note:<id>")
//   - Ensure(dim)     → GetOrCreateCollection (shape-free: chromem has no
//     fixed dim; a mismatch shows up as a search/Upsert error)
//   - Clear(dim)      → DeleteCollection + recreate (full re-embed boundary)
//   - Upsert          → Delete+AddDocument (chromem Add errors on duplicate
//     ids, so the delete-first makes it a true upsert and keeps idempotency)
//   - vectors are ALWAYS provided by the caller (RebuildEmbeddings /
//     embed-drainer computed them) — the collection's embeddingFunc is a
//     stub that must never run; chromem skips it when a document already
//     carries an embedding.
type Chromem struct {
	db      *chromem.DB
	collDir string // persistence directory ("" = in-memory)
	name    string // collection name, sanitised
}

const chromemCollectionDefault = "reponest_vecs"

// NewChromem opens (or creates) a persistent chromem store under dir. An
// empty dir yields an in-memory store — acceptable for tests, and for prod
// the caller passes a real directory next to dashboard.db.
func NewChromem(dir, collection string) (*Chromem, error) {
	name := sanitizeCollection(collection)
	if name == "" {
		name = chromemCollectionDefault
	}
	c := &Chromem{name: name}
	if dir == "" {
		c.db = chromem.NewDB()
		return c, nil
	}
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("vectordb: chromem dir must be absolute: %q", dir)
	}
	db, err := chromem.NewPersistentDB(dir, false)
	if err != nil {
		return nil, fmt.Errorf("vectordb: chromem open %s: %w", dir, err)
	}
	c.db = db
	c.collDir = dir
	return c, nil
}

func (c *Chromem) Name() string {
	if c.collDir == "" {
		return "chromem-go (memory)"
	}
	return "chromem-go"
}

// chromemNoEmbed is the embeddingFunc handed to GetOrCreateCollection. The
// Store contract only ever writes precomputed vectors, so if this runs it
// means a code path forgot to set Document.Embedding — fail loudly instead of
// silently emitting a zero vector.
func chromemNoEmbed(context.Context, string) ([]float32, error) {
	return nil, fmt.Errorf("vectordb: chromem embeddingFunc must never run (callers always provide precomputed vectors)")
}

func (c *Chromem) collection() (*chromem.Collection, error) {
	return c.db.GetOrCreateCollection(c.name, nil, chromemNoEmbed)
}

func chromemDocID(id int64) string { return "note:" + strconv.FormatInt(id, 10) }

func (c *Chromem) Ensure(dim int) error {
	if dim < 1 {
		return fmt.Errorf("vectordb: chromem ensure invalid dim %d", dim)
	}
	// chromem stores vectors as-is with no declared dimension; the dim check
	// happens naturally at search time (dot product length mismatch errors).
	if _, err := c.collection(); err != nil {
		return err
	}
	return nil
}

func (c *Chromem) Clear(dim int) error {
	if dim < 1 {
		return fmt.Errorf("vectordb: chromem clear invalid dim %d", dim)
	}
	if err := c.db.DeleteCollection(c.name); err != nil {
		return err
	}
	_, err := c.collection()
	return err
}

func (c *Chromem) Upsert(id int64, vec []float32) error {
	if len(vec) == 0 {
		return fmt.Errorf("vectordb: chromem upsert empty vector for note %d", id)
	}
	coll, err := c.collection()
	if err != nil {
		return err
	}
	ctx := context.Background()
	// chromem's Add rejects a duplicate ID, so delete-first: either this is a
	// fresh id (delete is a no-op) or a re-embed of an existing note.
	if err := coll.Delete(ctx, nil, nil, chromemDocID(id)); err != nil {
		return fmt.Errorf("vectordb: chromem upsert(delete) note %d: %w", id, err)
	}
	doc := chromem.Document{
		ID:        chromemDocID(id),
		Metadata:  map[string]string{"note_id": strconv.FormatInt(id, 10)},
		Embedding: vec,
	}
	return coll.AddDocument(ctx, doc)
}

// Delete removes one note's vector. chromem's Delete on an unknown id is a
// no-op, so this is idempotent without extra handling.
func (c *Chromem) Delete(id int64) error {
	coll, err := c.collection()
	if err != nil {
		return err
	}
	return coll.Delete(context.Background(), nil, nil, chromemDocID(id))
}

// Search returns the ids of the `limit` most-similar vectors, nearest first.
// chromem errors when nResults exceeds the collection size; we clamp instead,
// because "ask for 10, get 3" is the honest answer a small collection gives —
// the caller (hybrid search) already handles fewer hits than requested.
func (c *Chromem) Search(vec []float32, limit int) ([]int64, error) {
	if limit < 1 {
		return nil, fmt.Errorf("vectordb: chromem search invalid limit %d", limit)
	}
	coll, err := c.collection()
	if err != nil {
		return nil, err
	}
	if coll.Count() == 0 {
		return nil, nil
	}
	n := limit
	if n > coll.Count() {
		n = coll.Count()
	}
	results, err := coll.QueryEmbedding(context.Background(), vec, n, nil, nil)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(results))
	for _, r := range results {
		// Defensive parse: the metadata note_id is the source of truth, the
		// doc id is derived from it. A row that fails to parse is skipped
		// rather than aborting the whole search.
		if raw, ok := r.Metadata["note_id"]; ok {
			if id, perr := strconv.ParseInt(raw, 10, 64); perr == nil {
				out = append(out, id)
				continue
			}
		}
		if id, perr := parseChromemDocID(r.ID); perr == nil {
			out = append(out, id)
		}
	}
	return out, nil
}

// parseChromemDocID recovers the note id from a "note:<id>" document id.
func parseChromemDocID(docID string) (int64, error) {
	raw := strings.TrimPrefix(docID, "note:")
	return strconv.ParseInt(raw, 10, 64)
}
