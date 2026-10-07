// Package vectordb abstracts where note embeddings are stored and searched —
// the M3-A "axis B" (vector store), independent of the embedding provider
// ("axis A", hybrid.Embedder). Backends live in a registry (see store.go) and
// are listed by Kinds(); adding one = write a Store + Register("kind", factory),
// with no change at any call site. Currently shipped:
//
//   - local    : sqlite-vec inside dashboard.db (ADR-0013 default, pure Go, same
//     transaction as notes/FTS5). Always implicit — not a registry entry.
//   - qdrant   : a remote/self-hosted Qdrant server (HTTP REST).
//   - weaviate : a remote/self-hosted Weaviate server (HTTP REST, pre-1.24).
//
// Both remotes are for users who explicitly opt in (e.g. >1M vectors or
// multi-client sharing).
//
// Resolution falls back to local whenever the remote store is unset, is an
// unknown kind, fails a connectivity probe, or a call errors — mirroring
// ADR-0012's "never make search return less than before". Config lives in the
// service layer; this package takes plain values so it stays free of the
// config/db-schema plumbing.
package vectordb

// Store is a vector index: ensure its shape, rebuild, write one vector, and run
// k-NN returning note ids best-first. Implementations must be safe for the
// rebuild loop (Ensure then Clear then many Upsert) and for search-only use.
type Store interface {
	// Name is a stable label for logging ("local-sqlite-vec", "qdrant").
	Name() string
	// Ensure creates/repairs the index for the given dimension.
	Ensure(dim int) error
	// Clear empties all vectors (full re-embed), (re)shaping to dim if needed.
	Clear(dim int) error
	// Upsert writes/replaces one note's vector.
	Upsert(id int64, vec []float32) error
	// Delete removes one note's vector. Required by incremental indexing: a
	// deleted note whose vector survives keeps being recalled by search, and the
	// only other way to clear it is a full rebuild. Implementations must be
	// idempotent — the caller cannot know whether a vector was ever written.
	Delete(id int64) error
	// Search returns the ids of the `limit` most-similar vectors, nearest first.
	Search(vec []float32, limit int) ([]int64, error)
}
