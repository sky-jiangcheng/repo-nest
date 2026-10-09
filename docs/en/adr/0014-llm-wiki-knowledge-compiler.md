# ADR-0014: LLM Wiki knowledge compiler — compile at ingest, gather evidence at query time

- Status: Proposed (this record fixes direction and sequencing only; no code shipped. Promotion criteria at the end)
- Date: 2026-10-07
- Relates to: ADR-0003 (FTS5), ADR-0007 (context / handoff), ADR-0011 (multi-agent memory importers), ADR-0012 (semantic search), ADR-0013 (vector store), ADR-0010 (privacy-off-by-default discipline); TODO **M6**; starting version v1.15.1. (Chinese original: `docs/adr/0014-*.md`.)

## Context

At v1.15.1 the knowledge layer is a note store with **rich write channels, a very thin structure, only partially fused retrieval, and a passive consumer side**. All four imbalances are evidenced:

1. **Writes involve no processing.** Humans write Markdown, five external agent-memory sources are upserted idempotently (`service/plugin.go:114-120`, only claude is automatic), and agents write back through MCP `notes_create` / `handoff` — all three channels move raw text. Nothing anywhere extracts, integrates, or flags contradictions.
2. **Flat structure.** `project_notes` carries only title / tags / kind / pinned / source (`internal/db/db.go:131-144`), all hanging off the project: notes **do not reference a repository**, and notes **never reference each other** (no backlinks, no wiki-links, no link table anywhere — zero grep hits). It is a tagged list, not a knowledge graph.
3. **Partially fused retrieval.** FTS5 trigram covers notes + todos (`db/migrate.go:27-38`, `db/search.go:21-79`), but semantic RRF applies to notes only and is off by default (`service/search_semantic.go:15-30`, `service/search.go:27`; `SearchAll` does no semantic fusion, `search.go:30-44`). `repo_meta` (tech stack / README / dependencies / contributors, `db/migrate.go:79-88`) and the commit log **are not in the same searchable surface at all**.
4. **The consumer side gathers no evidence.** The desktop `AskAI` context is "10 notes, each truncated to 500 bytes" (`service/ai.go:380-436`, specifically `ai.go:431`) — no retrieval ranking, no streaming, no tool calls, no citations. **It is neither RAG nor compilation; it just stuffs a slice of the store into the prompt.** Implementing W2 also corrected this document's wording: those ten are not the *most recent* notes but the **oldest** ones — `db.ListNotes` orders `pinned DESC, sort_order ASC, created_at ASC, id ASC`, and `aiProjectContext` takes the first ten. The more notes a project accumulates, the less likely anything recently written is to reach the prompt. Ironically the consumer surface has long been agent-ready: 13 MCP tools plus `/api/rpc`, which reflects every Wails binding (`cmd/mcp/main.go:72`, `internal/httpapi/rpc.go:12-45`).

That contrast *is* the opportunity: **evidence-gathering is already sold to external agents but never used in-house.**

## References (what to copy, what is deliberately not copied)

- **Karpathy's original LLM Wiki gist** (`gist.github.com/karpathy/442a6bf555914893e9891c11519de94f`). The core is not a retrieval algorithm but the **moment of compilation**: RAG retrieves fragments at query time and never accumulates understanding; an LLM Wiki merges each new source **into a persistent, interlinked Markdown store** at ingest time — updating entity pages, revising topic syntheses, flagging where new data contradicts old claims. Three layers: raw sources (read-only), the wiki (LLM-owned read/write), and `schema.md` (the discipline that turns a generic chatbot into a maintenance bot). Three operations: ingest / query / **lint** (contradictions, stale claims, orphan pages, missing cross-references, data gaps). Two navigation artifacts: `index.md` (content catalog, updated on every ingest, read first at query time) and `log.md` (append-only, greppable timeline of what happened). **It states its own ceiling**: index navigation holds up to roughly 100 sources / a few hundred pages, beyond which real search is required.
- **`nashsu/llm_wiki`** (desktop implementation, ~20k stars) supplies the copyable directory and engineering detail: `purpose.md` / `schema.md` / `raw/sources/` / `wiki/{index,log,overview}.md` plus `entities/` `concepts/` `sources/` `queries/` `synthesis/`; pages are Markdown + YAML frontmatter with **no database**; a single ingest can touch **10-15 pages**; SHA256 skips unchanged files, two-step chain-of-thought ingestion, a **serial** queue with crash recovery and retries; good answers get filed back into `wiki/queries/` so explorations compound too.
- **`TencentCloud/TencentDB-Agent-Memory`** (~28k stars; the memory backbone shared by WorkBuddy / CodeBuddy / OpenClaw / Hermes / DeepSeek Harness, and it credits Karpathy explicitly). Two things it adds on top of the wiki are exactly what RepoNest should copy: **layered distillation** — L0 raw transcripts → L1 atomic facts → L2 scenarios → L3 persona, where retrieval uses "L2/L3 to bootstrap context, and only fall back to L1/L0 with BM25 + vector + RRF when a specific fact is needed", capped by a **triple budget of item count / character quota / timeout** so memory cannot devour the context window; and **asset-ised governance** — Chat Memory / Skill / LLM-Wiki / CodeGraph all register as Memory Assets carrying ownership / version / status / visibility (private|team|restricted) / usage counts / agent bindings.

## Decision

### 1. Structure first, generation second

The first step contains no LLM call at all: add a page layer and a link layer — `wiki_pages` (four kinds: `entity` / `concept` / `source` summary / `synthesis`) + `page_links` (traversable in both directions) + a note-to-repository association. **Without this graph, "one ingest touches 10-15 pages" is not even expressible in the data model**; starting with generation yields text nobody can navigate.

Groundwork already exists: `GenerateLLMsTxt` is effectively the embryonic index (project catalogue + recent notes + codebase summary, `internal/service/llm.go:16-58`, 200-note scan limit). What is missing is the page layer behind it, not yet another catalogue.

### 2. SQLite stays the single source of truth; the file tree is read-only export, never two-way sync

This is where we deliberately part with all three references, so it must be written down. They are all built on "the wiki is a git repo of Markdown files", using Obsidian for reading and graph view. RepoNest inverts it: **in-database structure + a one-way export bypass** (`reponest wiki export` renders a Markdown tree with frontmatter and wikilinks, plus a manifest).

Reason: the moment notes have two writers (a file editor and the in-app editor), conflicts, de-duplication and `note_versions` (currently up to 50 snapshots per note, `db/migrate.go:155-182`) all need a second implementation. **A one-way export already captures the two genuine benefits — Obsidian's graph view and "review what the LLM changed in git" — without taking on double-write costs.** If real demand for two-way sync ever appears (a user editing back from Obsidian), a separate ADR supersedes this one.

### 3. The consumer side comes before the generator side

Turn `AskAI` into actual evidence-gathering: read the index to locate candidate pages → rank pages with FTS5 + vector RRF → read them → answer **with citations** → let a good answer be filed back as a new page (Karpathy's query loop). Add streaming, and replace "10 notes × 500 bytes" with the item / character / timeout triple budget — the current constant neither saves tokens nor preserves fidelity.

Why this ordering: it improves an existing feature without touching the data model, and it is the verification ground for step 1 — whether a page layer pays off shows up fastest in answer quality. Measure with the existing `internal/search/abeval` (Recall@k / NDCG@k + `Compare` delta + threshold gate).

### 4. Compilation at ingest: opt-in, one source at a time, review before indexing

Upgrade importers from "upsert the external Markdown as a note" to "read source → extract → create/update pages → update index + append log → flag contradictions with existing notes". Three constraints: every LLM write carries `source='llm-wiki'` (the field already exists) and defaults to **pending review**, entering the index only after a human approves; one source at a time with the human in the loop (Karpathy works this way himself); and the whole chain is off by default, continuing the ADR-0010/0012 discipline of "sensitive capability off by default, documented".

**Blocking prerequisite:** `auto_import` then defaulted to `"1"` (`db/migrate.go:452`), so startup ran `ImportAll` over every automatic source. The compiler would amplify this existing privacy risk (raw text in the store → a crop of derived LLM synthesis pages), so tighten the default and the per-source gate **before** compilation.

> **Closed (schema v15, 2026-10-07)**: implementing it surfaced a deeper problem than assumed — `db.GetConfig` maps `sql.ErrNoRows` to `("", nil)` while the startup test was `v != "0"`, so **"never configured" was itself equivalent to consent**; the default value was not the bug. The fix inverts the predicate to `== "1"` (making the safe state the default), seeds `"0"`, and adds a one-time migration normalizing legacy databases. W3 still needs its own per-source gate: v15 only removes "import at startup".

### 5. Lint is mandatory maintenance, not an optional extra

A scheduled lint pass checks five things (contradictions / stale claims / orphan pages / missing cross-references / data gaps) and its findings **land in `project_todos`** (reusing the existing table rather than inventing a feedback surface). Hard constraint: the LLM may only suggest, never auto-rewrite a page — the only effective barrier against "hallucinations compiled into apparently established knowledge". The human adjudicates by ticking or keeping the todo.

### 6. Map layered memory onto existing tables; do not build a remote hub

L0 = session transcripts (the capture pipeline already exists) / L1 = atomic notes (`project_notes`) / L2 = project scenarios (`BuildProjectContext` is already this shape, with handoff-first ordering, `internal/service/context.go:94-120`) / L3 = cross-project stable persona (not yet scoped). Retrieval bootstraps from L2/L3 and gathers from L1/L0 instead of injecting everything.

Externally, still only the two existing surfaces: MCP tools and `/api/rpc`. **Do not build or bind to a remote Memory Hub** — RepoNest is local-first (the scoop manifest literally says "Local-first code project context base"). If interoperation ever matters, the direction is RepoNest acting as **one local asset source / audit panel** for such a hub, and only once OMP stabilises (our own export is still marked PROVISIONAL and the import side is deliberately unwritten, `internal/service/omp_export.go:13-19`).

## Candidate matrix

| Option | Distance from today | Value | Main risk |
|---|---|---|---|
| Stronger RAG only (pull commits / repo_meta / todos into one searchable surface + let `AskAI` use vectors) | Nearest; a subset of step 3 | Fast, no migration | Understanding still does not accumulate: every query rebuilds from scratch, nobody flags contradictions |
| File-tree wiki (copy nashsu / the Obsidian ecosystem) | Requires replacing the current SSOT | Free graph view, diff-friendly, git-backed | **Two writers**; `note_versions` / FTS5 / vector index all need a second implementation; desktop UX regresses |
| **This decision: in-database structure + one-way export + layering + opt-in compilation** | Medium | Compounding understanding, navigable, auditable, no SSOT migration | Wrong page taxonomy hurts later; high token cost (see Consequences) |
| Adopt Tencent's Hub as the backend | Far | Team sharing / ACL / Skill assets for free | Breaks local-first; adds an external process and protocol churn; privacy surface balloons |

## Consequences

**Positive**: knowledge finally carries "who wrote it, where it came from, what it contradicts"; `AskAI` moves from "inject the head of the store" to "gather evidence, cite it"; the MCP surface gains navigation far stronger than 13 flat tools; `note_versions` + `source` turn out to be a ready-made audit and rollback base for LLM writes (those two fields were built for exactly this day).

**Negative (not sugar-coated)**:
- Token cost far above naive indexing — one ingest is several LLM calls that may touch a dozen pages; when a local Ollama cannot carry it, this path degrades.
- Hallucinations and omissions get compiled into conclusions that look established, which is **harder to spot than in RAG** (the error is in the index and the synthesis, not in the source text). Lint plus review-before-index is the only barrier.
- Serial ingestion makes the first import of a large store slow; beyond a few hundred pages index navigation necessarily degrades and vectors must take over — conveniently sqlite-vec is already in-database and zero-CGO, so this one is an advantage rather than a cost.
- The privacy surface widens: see the prerequisite in decision 4.
- New migration + new config keys + new UI: three layers move at once; this is not a one-sprint job.

**Open questions**: ~~whether four page kinds suffice~~ (→ **decided by W1: five kinds**); ~~whether the `schema.md` equivalent is global or per-project~~ (→ **decided 2026-10-09: global default + per-project override**; the kind vocabulary and the safety contract (three ops, budgets, review-before-index) stay global and non-overridable, the override surface is descriptive discipline only, and the SSOT lives in SQLite, not the export tree. Full rationale in TODO M6); the relationship with `dsh-plugin-reponest` (should the export bypass feed a dsh session?); whether the semantic fusion scope should extend from notes to `wiki_pages` once page volume grows (currently constrained by ADR-0012's "fuse notes only").

> **Two corrections made while implementing W1 (2026-10-08, schema v16)**
>
> 1. **Four kinds became five.** Decision 1 listed entity / concept / source / synthesis, but the query loop promised by Decision 3 ("file good answers back as new pages") needs a destination, and both Karpathy's pattern and the `nashsu/llm_wiki` reference keep a `queries/` area. Without it an answer can only be written back as an ordinary note. The implementation therefore has five kinds, guarded twice over by a SQLite `CHECK` and the Go enum.
> 2. **"Reversible" needs self-healing to mean anything.** Migrations are stamped, so v16 runs once, while a derived layer is by definition allowed to be dropped and rebuilt (Decision 5 keeps the SSOT in `project_notes`). Building the tables only inside the migration would leave a database stamped v16 with no tables after a single `DropWikiSchema`. `InitDB` now calls `EnsureWikiSchema` unconditionally, under the same "safe to call on any database" contract as `EnsureFTSIndex`. This was **caught by the reversibility test**, not thought of at design time: the test failed first.
>
> 3. **W1b shipped too, with the command name corrected**: `reponest wiki export` as written above does not exist — the root `reponest` binary is the Wails app with no subcommand dispatch. What actually landed is a separate `reponest-wiki-export` binary (`cmd/wiki-export`), and it does two things the plan did not call for: stale files left by deleted pages are reported rather than removed (deleting them would claim the tree belongs to the exporter, the very stance Decision 2 rejects), and `[[wikilinks]]` typed into page bodies that point at missing pages are counted (page-to-page edges cannot dangle, FK cascades cover them; body links are the only rot path) — that count is W4 lint's input.
>
> 4. **W4 split the "five checks" into two tiers when it landed.** The original text treated lint as one kind of action, but three of the five checks (orphans, missing cross-references, data gaps) are decidable in SQL while two (contradictions, stale claims) need a model, cost tokens and can be wrong. So the structural tier runs any time and the model tier sits behind its own `wiki_lint_llm` switch, off by default, with `llm_note` separating "found nothing" from "not enabled". Two adoption rules the plan did not state were added: a model-supplied `page_id` becomes a finding only if that page exists, and a reply that is not parseable JSON yields no findings at all.

Also fixed a brittle assertion: the v15 migration test asserted the schema version as exactly 15 and went red the moment v16 landed. The invariant is "v15 has run" (`>= 15`); otherwise every migration forces a rewrite of an old test.

## Promotion criteria (Proposed → Accepted)

1. The step-1 migration runs on a real database and is **reversible** (migration test, at the rigour of `migrate_fts_repair_test`).
2. Step 3 delivers a delta on a real labelled query set via `cmd/abeval`, proving "evidence beats stuffing" is measured, not felt.
3. The privacy prerequisite of step 4 (`auto_import` default + per-source gate) is closed on its own.
4. A conclusion exists on the four page kinds and the schema scope.

The task breakdown for these lanes lives in `TODO.md` under **M6**.
