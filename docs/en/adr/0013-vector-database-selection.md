# ADR-0013: Vector store selection (sqlite-vec) and onboarding guide

- Status: Accepted (default local sqlite-vec + `VectorStore` registry with Qdrant & Weaviate remote backends, auto-fallback to local; `cmd/vector-init` guide)
- Date: 2026-10-03
- Relates to: ADR-0003, ADR-0012, TODO M3. (Chinese original: `docs/adr/0013-*.md`.)

## Background

M3 needs vector storage (axis B), independent of the embedding provider (axis A). A selection draft claimed "sqlite-vec needs CGO" and "the vector store isn't built yet" — both corrected: the repo uses **`modernc.org/sqlite/vec` (pure Go, CGO-free, verified)**, and the store layer (`internal/db/vecindex.go`) already exists.

## Decisions

1. **Default = local `sqlite-vec`** (same `repotory.db` file, same transaction, zero extra process).
2. **Remote is opt-in via a pluggable `VectorStore` registry** (`internal/search/vectordb`: `Register(kind, factory)` + `Kinds()`; `Open` selects by `vector_store`, and **falls back to local on unknown kind / unreachable remote**). Backends: `local`, `qdrant` (REST), `weaviate` (REST + GraphQL nearVector). Contracts verified against **live local containers** before coding (httptest + build-tag `qdrantlive`/`weavialive`).
3. **Zero-CGO is a hard filter**: LanceDB / go-libsql need CGO → not default candidates.
4. **`cmd/vector-init`** guides setup (self-check vec, store health check, choose embedding provider Ollama-local / remote / skip + vector-store choice, then point the user to Settings). `semantic_search` stays off.
5. **Interop outlook**: watch **OMP (Open Memory Protocol)** — a vendor-neutral AI-memory spec (v0.4, pre-1.0, early) — as a future import/export contract to replace per-tool format reverse-engineering; a provisional `ExportMemoryJSON` seam is shipped, import side deferred.

## Consequences

- Positive: candidates and config are flexible (new remote backend = one `Store` + one `Register`, callers unchanged); defaults stay local, zero-CGO, offline.
- Caveats: single-file vector scale ceiling (fine for a personal corpus); live smoke tests are build-tag gated (not in CI); cloud Qdrant/Weaviate/Pinecone and the offline-only chromem-go/bleve additions remain future work.
