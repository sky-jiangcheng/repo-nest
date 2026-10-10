# ADR-0018: Knowledge-graph foundation — a controlled relation vocabulary makes the graph queryable, graph-aware evidence makes it read

- Status: Proposed (**lane 1 code and contract tests have landed**: schema v21 relation vocabulary — the CHECK is derived from `wikiRelations` so the DDL and the Go set are one source of truth — plus compiler relation selection, approve-time `mentions` extraction, and the three `lintRelationShape` checks; `go test ./internal/db ./internal/service ./internal/integrity` all green. **The only unmet promotion criterion is lane 1's #1, "rehearse on a real-DB copy + retain a read-only dump of the old edges"** — it runs against your actual `dashboard.db` copy, which the agent does not do on your behalf, so lane 1 is not yet marked Accepted. Lane 2 (page vectors + graph-aware evidence, schema v22) is gated behind lane 1 being Accepted + a passing Phase 0 delta and has not started.)
- Date: 2026-10-09
- Relates to: [ADR-0014](0014-llm-wiki-knowledge-compiler.md) (this delivers the second half of its "build structure first" promise — the structure exists but must be queryable — gives its lint "data gap" check a concrete shape, and closes its open item on whether semantic fusion extends to `wiki_pages`), [ADR-0015](0015-w3-minimal-compile-loop.md) (this does not violate "the compiler never edits an approved page" — new writes touch only edges and the pending surface), [ADR-0016](0016-async-batch-jobs.md) (if lane 2 is slow, reuse the one-table-two-kernels job), [ADR-0012](0012-semantic-search.md) (RRF fusion), [ADR-0013](0013-vector-database-selection.md) (zero-CGO vector store), [ADR-0010](0010-session-auto-capture.md) (the discipline of shipping sensitive capability default-off); TODO **M6** closure + a new **M7**; starting schema v20
- Companion plan: `RepoNest-知识图谱实施计划-2026-10-09.md` (this ADR = its Phase 1 + Phase 2; Phase 3 entity resolution and Phase 4 community summaries / temporal graph each get their own ADR-0019/0020/0021). (Chinese original: `docs/adr/0018-*.md`.)

## Context

ADR-0014's first decision was "build the structure before letting the LLM write", justified as "without this graph, 'one ingest touching 10-15 pages' isn't even expressible in the data model". That graph now exists (`wiki_pages` + `page_links` + `note_pages`, since schema v16), but **existing ≠ usable**. Three code-level facts show it is currently write-only:

1. **Relations are free-form strings.** `page_links.relation` is unconstrained text defaulting to `'ref'` (`internal/db/wiki.go:61-70`, no `CHECK`). The compiler stores whatever the model emitted verbatim, falling back to `"compiled-from"` when empty (`internal/service/wiki_compile.go:377`); answer-filing writes `"cites"` (`wiki_evidence.go:292`). **Nothing anywhere constrains the relation value**, so the "graph" cannot be queried semantically — you cannot ask for "all `depends` edges" because no one guarantees the word is spelled consistently.
2. **Evidence gathering never reads the graph.** `GatherEvidence` (`wiki_evidence.go:91`) only calls `db.SearchWikiPages` (FTS) and `s.SearchNotes`; **it never calls `WikiEdgesFrom`/`WikiEdgesTo`**. Edges are read in exactly three places for one-hop rendering: export backlinks, lint structural checks, and compile-prompt context. The user can see the graph in Obsidian, but AskAI treats it as non-existent when answering.
3. **Pages have no vectors.** The vec0 index `note_embeddings` in `internal/db/vecindex.go` is **notes-only** (`KnnNoteIDs`, `PutNoteEmbedding` all keyed by note rowid). `wiki_pages` has no vector face, so ADR-0014's open item "extend semantic fusion to wiki_pages" is still undone. A repo-wide grep for `WITH RECURSIVE|pagerank|leiden|louvain|community|betweenness|transitive` returns **zero hits**.

Net: the Wiki's "knowledge net" currently has only its **nodes** working (pages are FTS-searchable); both remaining legs — **edges** and **vectors** — are unconnected. The gap to ADR-0014's vision ("navigable, evidence-gathering") is not the data model but the layer that uses it. This ADR is that layer, and most of it is doable in pure SQLite + Go with zero new dependencies.

## References (copy the mechanism, not the implementation)

- **Karpathy's LLM Wiki**: `index.md` is human navigation; `[[wikilink]]` is the edge. He concedes "index navigation works within a few hundred pages; larger needs real retrieval" — RepoNest's corpus will eventually cross that line, so "queryable edges" is the prerequisite for "traversable retrieval".
- **Microsoft GraphRAG**: two mechanisms worth borrowing as ideas — **local search** (a personalized random walk over the graph from retrieval-hit entities to pick evidence, ≈ this ADR's PPR) and **community summaries** (community detection + per-community summaries to fill the empty synthesis layer — **not done here**, see Phase 4). Its stack binds Neo4j + Leiden + map-reduce global summaries, which breaks local-first / zero-CGO / single-SQLite; **borrow the criteria, not the stack**.
- **TencentDB-Agent-Memory**: its layered principle "fall back to BM25 + vector + RRF only when you need a concrete fact" is exactly lane 2's "vector coarse recall → graph fine ranking" stratification.

## Decision (two halves, promoted separately)

### Lane 1 (MVP, schema v21): turn edges from strings into queryable relations

**1. A controlled relation vocabulary of 8 types**, frozen as `ref` (default generic reference) / `part-of` / `depends` / `implements` / `documents` / `supersedes` / `contradicts` / `mentions`. Too few (e.g. 3) and lint can't detect cycles/inversions; free text and there is no graph at all. The vocabulary lives as a Go enum in `internal/db/wiki.go` (new `Relation*` constants + `ValidRelation`) and is double-guarded by a SQLite `CHECK` (same "CHECK + Go enum" technique that already protects the 5 page kinds).

**2. Adding a `CHECK` to `page_links` requires a full table rebuild.** SQLite cannot `ALTER` a column-level `CHECK` (hard constraint), so v21 = create a new table with the `CHECK` → `INSERT … SELECT` while **normalizing existing illegal relation values to `ref`** (never drop an edge, only relabel) → drop old → rename. Idempotent, and the **reversibility test requires** byte-equal `sqlite_master` plus an equal edge multiset before/after. A read-only dump of the old edges must be producible before migration (see Consequences · Negative).

**3. The compiler picks a word from the enum.** Rewrite the JSON contract literal at `wiki_compile.go:227-228` so `"relation":"…"` is a closed enum plus definitions/examples; change the default at `:377` from `"compiled-from"` to `"ref"`; `LinkWikiPages` (`wiki.go:488`) **downgrades an unknown relation to `ref` and logs, rather than erroring** (dirty values must not break a write). All existing guards stay: `add_link` requires at least one end to be this run's product (`ownedByCompile :403`), budgets, pending-only writes, never edit approved pages.

**4. Body `[[wikilink]]` is extracted into `mentions` edges at approve time.** Export already parses body links and counts `dangling` (`wiki_export.go:50,243`). Do the reverse: for body links that **resolve to an existing page**, `LinkWikiPages(…, 'mentions')` — `UNIQUE(from,to,relation)` + `ON CONFLICT DO NOTHING` is naturally idempotent. Unresolved ones stay "report only" (dangling semantics unchanged). **The trigger is approve** (only human-approved pages enter the graph), reusing the `ApproveWikiPage` path — no new job, and it is not part of the compiler's automatic write face.

**5. A structural check for the vocabulary goes into lint.** Add `lintRelationShape` to `RunWikiLint` (`wiki_lint.go :125`), **following `lintMissingRefs`' "prefetch the edge set to avoid O(n²) queries"** pattern (`:192-202`), and scope the edges to the project under review. The three checks (finalized at implementation, all unambiguously decidable): **`supersedes` inversion** (the superseder's `updated_at` predates the superseded's), **`part-of`/`depends` cycles** (composition and dependency should be acyclic; a colored DFS finds back-edges), and **`contradicts` one-sided** (a contradiction is symmetric, so a missing reverse edge is flagged). **Output still only writes `project_todos`** (`fileFindings :389`), never mutating pages.

> **One replacement made at implementation**: the draft's third check was "`documents` pointing at a non-source", which turned out to be either vacuously true or dependent on a kind↔relation matrix that ADR open-question #1 explicitly defers — an undecidable pseudo-check. It was replaced by **`contradicts` symmetry**, which needs no cross-field convention, is purely graph-internal, and actually catches "half-marked" contradictions. None of the three checks introduce a kind↔relation constraint, honoring the "edge types only" commitment.

**Lane 1 explicitly does NOT**: freeze a full kind↔relation matrix (define edge types first, defer the matrix until real data exists), batch-rewrite existing legal edges (the migration only normalizes illegal values), or add edge weights/properties (that is lane 2).

### Lane 2 (schema v22, gated): let evidence gathering read the graph

**6. An independent page vector space.** Mirror `note_embeddings`: a new `page_embeddings` (vec0, keyed by `wiki_pages.id`, **embedding approved pages only**) + `page_embeddings_meta`. **It must be a separate table** — `wiki_evidence.go:104-108` records that `FuseRRF` over mixed note/page ids collides (it returns bare int64 lists, so page#7 and note#7 are indistinguishable). Incremental updates mirror v14: a `page_embed_dirty` queue + triggers (body text actually changed / status moving into approved sets dirty, moving out deletes the vector). **Gated behind the existing `semantic_search`** (that key is the user's consent to send text to the endpoint); no separate embedding switch, so "main switch on, page-embed off" cannot silently leave a stale index. The store goes through the `vectordb.Store` registry (local=sqlite-vec default, remote unreachable falls back to local). ⚠️ `vectorStore()` is memoized (`search_semantic.go:71`); changing `vector_store*`/`embedding_*` or hitting a store error must `invalidateVectorStore()`, else page embedding silently uses a stale endpoint.

**7. Graph-aware evidence = 1–2 hop expansion + PPR ranking.** The insertion point is unique and already reserved: `evidencePages` in `wiki_evidence.go` (`:163-182`), whose comment (`:104-108`) literally says "position interleaving is used *until pages have their own vector space*" — that is the slot.
- **Expansion**: from the seed pages that FTS/page-vector hits return, a `WITH RECURSIVE` walks 1-2 hops along a **relation whitelist** (`ref`/`part-of`/`documents`; **not `contradicts`** — contradiction is shown to the user, not folded into the same evidence block) and adds neighbors to the candidate set.
- **Ranking**: a pure-Go personalized PageRank (PPR) with reset over the directed `page_links` subgraph. The reset vector = hit pages + (hit notes projected onto their pages via `note_pages`). Transitions follow the relation whitelist and edge weights. Hundreds-to-thousands of pages converge in milliseconds (~15 iterations), well inside `EvidenceBudget.Deadline = 3s`. It replaces the current bm25 ordering.
- **Fusion**: the notes side keeps `SearchNotes`→semantic RRF; page-vector hits and page FTS hits are `hybrid.FuseRRF`'d **in the page-dedicated id space**, then the page and note results are **interleaved as separate typed columns** (never put across types in one RRF call, which is the collision trap).
- **Switch** `wiki_graph_search` (new bool key, default off): when off, `evidencePages` keeps the current FTS order with **zero behavior regression**; when on, neighbors and PPR take effect.

**Lane 2 explicitly does NOT**: external graph DBs (Neo4j/Memgraph/KuzuDB), RDF/OWL/SPARQL, GNNs — all violate local-first + single-SQLite + zero-CGO (ADR-0014 decision 2, ADR-0017). Pure SQL recursion + Go PPR is enough up to tens of thousands of pages; if an external store is ever warranted, the `vectordb` "pluggable + fall back to local" seam already shows how, and that is its own ADR.

### Prerequisite (gates lane 2's measurement, not lane 1's coding)

Phase 0 — a real labelled query set + `abeval` delta (`cmd/queryset`/`cmd/abeval`, scaffolding ready, annotation is inherently human). Lane 1's value is **structural** (edges queryable, lint detects cycles) and needs no recall metric to justify it; but lane 2's core claim — "graph-aware evidence beats pure FTS" — **must** show a positive evidence→evidence+graph delta on the same `queries.jsonl`, otherwise it is "a machine that computes more", not "better evidence".

## Candidate matrix

| Option | Distance from today | Value | Main risk |
|---|---|---|---|
| Lane 1 only (vocab + wikilink + lint), leave retrieval alone | Nearest, one v21 | Graph instantly queryable, lint gains structural criteria, no new outbound surface | Edges still don't participate in answers; the Obsidian graph and AskAI keep ignoring each other |
| **This decision: lane 1 structure + lane 2 consumption, both default-off** | Medium | Graph is both queryable and read during answers; closes an ADR-0014 item | Lane 2 moves three layers, not a single sprint; PPR may pull in low-quality neighbors |
| Skip lane 1, jump straight to page vectors | Looks like a shortcut | Fast | Relations are still free text — you can't say which edges PPR walks, the whitelist is undefined, and the collision trap runs naked |
| Copy GraphRAG onto Neo4j | Far | Team sharing, mature algorithms | Breaks local-first / single-store / zero-CGO; unnecessary at a few-hundred-page scale; privacy surface spikes |
| Also do community summaries here (Phase 4) | Medium | Fills the empty synthesis layer | Touches `compile_jobs.kind`'s CHECK, whose new value needs a full table rebuild; should be separately scheduled with its own ADR |

## Consequences

**Positive**: edges go from noise to queryable semantics (`WHERE relation='depends'` is meaningful for the first time); AskAI upgrades from "flat text evidence" to "evidence over a graph"; lint's "data gap" check becomes a structural fact rather than a heuristic; ADR-0014's "extend fusion to wiki_pages" is closed; the page and note layers share one vector backend and one gate, no second piece of infrastructure.

**Negative (no sugarcoating)**:
- **v21 rewrites the relation on existing edges** (illegal → `ref`) — this mutates the user's DB irreversibly; it drops no edge but relabels. Mitigation: the migration only "normalizes illegal, keeps legal", has a byte-level reversibility test, and **is rehearsed on a real-DB copy with an old-edge dump retained before shipping**.
- Too narrow a vocabulary drops nuance, too broad goes unused; 8 types is a starting point, and widening it is another rebuild.
- Lane 2 **widens the outbound surface**: with `semantic_search` on, page bodies (possibly containing code snippets and session excerpts) go to the embedding endpoint. Docs must state "what this sends out" exactly as ADR-0015 decision 7 does.
- PPR neighbors carry noise: three guards (approved-only + relation whitelist + the triple budget) suppress but do not eliminate it; abeval delta is the final arbiter.
- One ADR, **two promotions** — readers must track which half is approved, hence the status line spells out each half's schema and gate.

## Open questions (with recommendation)

| # | Question | Recommendation |
|---|----------|----------------|
| 1 | Include `mentions` (auto-extracted from body wikilinks)? | **Yes** — the only deterministic, model-free edge source; build graph density first |
| 2 | Legacy rows with wildly varied relation values | **Normalize into `ref` and dump the old values read-only** — never drop, never guess a mapping |
| 3 | PPR vs bm25: who orders? | **PPR orders, bm25 demotes to seed recall**; with `wiki_graph_search` off, keep today's bm25 order |
| 4 | Reuse the note embedding model/dim for pages? | **Reuse provider and dim**, but keep the index physically separate (collision trap); each keeps its own meta |
| 5 | Expansion hop count | **Default 1 hop**; open 2 only after abeval proves the gain |

## Promotion criteria

**Lane 1 (→ lane 1 Accepted; may be promoted before lane 2):**
1. v21 reversibility — `sqlite_master` snapshot + edge multiset equal before/after rebuild, legal relations untouched, illegal values deterministically normalized to `ref`; rehearsed once on a real-DB copy with the old-edge dump retained;
2. Contract test: `LinkWikiPages` downgrades unknown relations without erroring; compiler output lands 100% within the 8-type enum;
3. Approve-time `mentions` extraction is idempotent (rerun adds no edges) and **only touches approved pages** (pending-page body links don't enter the graph);
4. `lintRelationShape`'s three checks (inversion / cycle / one-sided contradiction) each fire and **only write `project_todos`, never mutate pages** (folded into the `TestWikiLint_NeverMutatesPages` invariant);
5. Vocabulary frozen + bilingual docs (`docs/features/knowledge.md` "edge types" section, both locales) + this ADR's status line updated.

**Lane 2 (→ lane 2 Accepted, prerequisite = lane 1 already Accepted):**
6. **On Phase 0's `queries.jsonl`, evidence+graph beats evidence on Recall@k/NDCG@k with a positive delta that passes the `abeval` gate** — mechanism correctness ≠ real gain; without this, lane 2 cannot be Accepted;
7. Separate page vector space: `page#N` and `note#N` coexisting must not collide in RRF (dedicated regression); with `wiki_graph_search` off, `evidencePages` output is byte-identical to today (no silent change on the default path);
8. PPR pure-function tests: adjacency construction, reset-vector convergence, and time bounded by `EvidenceBudget.Deadline`;
9. Reversibility of `page_embeddings` (it is a derived index — droppable + self-healed from `InitDB`, mirroring `EnsureWikiSchema`/`note_embeddings`), plus a "refuse to build when dim unknown" test (guessing width would drop and rebuild the whole vec0 index);
10. Bilingual docs (the widened outbound surface) + CHANGELOG + this ADR's lane 2 status updated.
