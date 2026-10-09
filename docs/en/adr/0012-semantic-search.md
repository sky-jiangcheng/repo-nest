# ADR-0012: Semantic search — vector recall to cover FTS5 blind spots, stay zero-CGO (M3)

- Status: Accepted-in-principle (vector storage zero-CGO verified; hybrid RRF core landed; C shipped default-on; A backend landed but default-off, gated on A/B eval + risk disclosure; B dropped)
- Date: 2026-10-03
- Relates to: ADR-0003 (FTS5), ADR-0013, TODO M3. (Chinese original: `docs/adr/0012-*.md`.)

## Background

FTS5 trigram + bm25 handles literal/substring well but has no semantic recall ("heap keeps growing" won't match "memory leak"). Constraint: the DB driver is `modernc.org/sqlite` (pure Go, **zero CGO**); a desktop cross-platform app must not reintroduce CGO.

## Decisions

1. **Vector storage/search is feasible at zero-CGO — verified.** `modernc.org/sqlite/vec` bundles sqlite-vec v0.1.9 as pure Go; live-verified under `CGO_ENABLED=0` (`internal/vecprobe` regression: vec0 build + KNN + `vec_distance_l2`). Only **local embedding generation** was the hard part.
2. **Embedding provider (axis A) = OpenAI-compatible `/v1/embeddings`**, not vendor-locked (OpenAI / Voyage / Jina / self-hosted Ollama all speak it). Pointing at **local Ollama** keeps data on-device **and** zero-CGO (model runs out-of-process) — resolving the local-vs-cloud dilemma. `hybrid.RemoteEmbedder` implements it. B dropped (no quality pure-Go inference runtime exists).
3. **Ship as hybrid + RRF, default OFF**: `hybrid.FuseRRF` merges FTS5 and vector recall; `service.fuseSemantic`/`RebuildEmbeddings` wire it, **off unless `semantic_search=1`** + endpoint configured; any failure degrades to lexical (never fewer results). `App.RebuildEmbeddings` binding.
4. **A/B eval gate before exposing to users**: `internal/search/abeval` (recall@k / NDCG@k + `Compare`) + `cmd/abeval` run lexical-vs-hybrid on a labeled query set and PASS/FAIL a `-min-recall` margin.
5. **Embeddings are a derived cache**: SQLite stays the source of truth; rebuild anytime.

## Landed + live-verified

`internal/search/hybrid` (+`RemoteEmbedder`), `internal/db/vecindex.go`, `internal/search/vectordb` (Local/Qdrant/Weaviate, ADR-0013), C (FTS OR-relaxation) default-on. Live smoke tests (`ollamalive`/`qdrantlive`/`aelive`/`weavialive`) ran against **real local Ollama + Qdrant/Weaviate**, incl. full "Ollama→Qdrant→fused recall". Remaining: A/B harness on real labeled queries and cloud-vendor verification. The settings UI now exposes the opt-in switch and rebuild action; incremental per-note embedding has landed.
