# ADR-0021: Temporal graph — giving pages a foundation you can go back to when an edit was wrong

- Status: Proposed (**design only, no code.** Promotion prerequisite: ADR-0018 lane 1 Accepted — the version table's migration discipline must be validated by a real-DB rehearsal first, same as ADR-0019/0020. This ADR is Phase 4b of the companion plan, a strategic item explicitly deferred; it **does not relax** ADR-0015's "the compiler never edits an approved page" — it only builds the foundation that a future, safe relaxation would need.)
- Date: 2026-10-10
- Relates to: [ADR-0014](0014-llm-wiki-knowledge-compiler.md) (the five page kinds; `note_versions` has been the notes' audit foundation since v8 — this ADR gives pages the same), [ADR-0015](0015-w3-minimal-compile-loop.md) (the "never edit an approved page" red line + `fileRevisionTodo`'s `compile-revision` todo — this ADR provides the only future path to relaxing that line safely), [ADR-0018](0018-knowledge-graph-foundation.md) (`supersedes` in the eight-type vocabulary is a ready-made temporal edge; `lintRelationShape`'s supersedes-inversion check can currently only compare `updated_at` wall clocks), [ADR-0019](0019-entity-resolution-merge.md) (merge execution re-points edges and equally benefits from "an edit you can go back from"), [ADR-0020](0020-community-synthesis.md) (its `synthesis-revision` todo and this ADR's "apply a revision" surface are the same UI slot); TODO **M7** follow-on; starting schema v21
- Companion: Phase 4b of `RepoNest-知识图谱实施计划-2026-10-09.md`. (Chinese original: `docs/adr/0021-*.md`.)

## Context

The page side has a gap the notes side closed back in v8: **no version history.**

- Notes: the `note_versions` table + `note_versions_snap` trigger (`internal/db/migrate.go:152-181`) — every change is snapshotted automatically, 50 kept per note, FK cascade; v11 further narrowed the trigger so it fires only when title/content/tags/kind actually change. The UI has a version list + LCS diff + one-click restore (the "Version history" section of `docs/features/knowledge.md`).
- Pages: `wiki_pages` carries only `created_at`/`updated_at` wall clocks (`internal/db/wiki.go:41-58`), and `UpdateWikiPage` (:698) overwrites in place — **a wrong edit has no way back.** When the compiler wants to revise an approved page, `fileRevisionTodo` (`wiki_compile.go:416-423`) can only file a `compile-revision` todo, which amounts to "human, you go edit it": the human edits by hand, again with no snapshot.

So the companion plan's Phase 4b names the temporal graph as **the prerequisite for letting the compiler safely revise existing conclusions** — without snapshots and rollback, the "never edit approved pages" red line can never be loosened, and the compiler stays frozen at "create-only" semi-automation. This ADR is that prerequisite, nailed down at the design level: version granularity, retention, and the relationship to lint first; loosening the red line later.

There is a second temporal debt: `supersedes` in the eight-type vocabulary is semantically a temporal edge, yet `lintRelationShape`'s inversion check can only compare `updated_at` wall clocks (`wiki_lint.go`), and wall clocks get polluted by any unrelated touch — which point in time "A supersedes B" refers to has no structural basis today. This ADR's v1 does not fix that (decision 6) but sets the direction.

## References (copy the mechanism, not the stack)

- **This repo's own `note_versions`**: the thing to copy — same database, same trigger pattern, same 50-row cleanup, same FK cascade, with LCS diff and the version UI already built. Pages and notes are peer knowledge carriers in this repo; there is no reason for the audit foundation to come in two flavors.
- **Bitemporal modeling (valid_time × transaction_time)**: the textbook answer — "when did this fact become true" recorded separately from "when did we learn it". Correct direction, but this repo has **no consumer** that needs to query by time interval; adopting it now doubles the write path for an imagined need. Not in v1; the direction stays in open question 5.
- GraphRAG / Neo4j temporal extensions: equally bound to an external stack, violating local-first / single-SQLite / zero-CGO (ADR-0014 decision 2, ADR-0017); borrow only the judgment that "time is a standard dimension of a knowledge graph," not the implementation.

## Decision

**1. `page_versions` mirrors `note_versions`; no second design.** Column-for-column alignment (`page_id / title / content / kind / created_at`, FK `ON DELETE CASCADE`, `(page_id, created_at DESC)` index); an `AFTER UPDATE ON wiki_pages` snapshot trigger plus a per-page 50-row cleanup trigger, both `IF NOT EXISTS` for idempotency. The schema number is **v23**, leaving v22 for lane 2's `page_embeddings` (open question 1) — "v22 = page vectors" is already written into several documents, so this ADR does not grab it. v23 is contested with [ADR-0019](0019-entity-resolution-merge.md) decision 6's optional `page_merges`; the tie is broken by "first migration to ship claims the number" (open question 1).

**2. Snapshot trigger surface: only when title/content/kind actually change.** Copy the v11 lesson (`migrate.go:200-210`): `status` transitions (pending→approved, reject), `source`, and wall-clock-only touches **do not snapshot** — review state is already queryable in the review queue, and mixing it into content history only fills the version list with meaningless snapshots. The `WHEN` clause compares columns one by one; correctness does not rely on application-layer discipline.

**3. Rollback = snapshot current first, then apply the old version; never delete versions.** "Restore to this version" is not DELETE history — the current content is first stored as a new version (it too was a real state that existed), then the old version's content is written back as current. This is the same discipline as this repo's `reject≠delete` and merge re-pointing without deleting pages: **history only grows.** When a page is deleted its versions go with it via FK cascade — that is the page disappearing as a whole, not cherry-picking history for deletion.

**4. The read trio aligns with the notes side; desktop + UI only.** `ListPageVersions` / `GetPageVersion` / page-level LCS diff (the same algorithm as `NoteDiff`, already built), exposed through thin `internal/app/bindings.go` wrappers plus a version sidebar on the Review/Knowledge pages. **No new MCP tools** (ADR-0015 decision 6: an agent that can edit history bypasses the human); rollback is a destructive write, triggerable only by a human in the UI.

**5. This ADR does not relax "the compiler never edits an approved page."** It only builds the foundation. The revision loop comes in two steps: v1 keeps humans editing by hand (but now with rollback and a pre-edit diff); "the machine drafts a revision → a human applies it in one click → it can be rolled back" is v2, to be decided on its own then, sharing the same "apply a revision" UI slot with ADR-0019's merge execution and ADR-0020's `synthesis-revision`. **Splitting "loosen the red line" from "build the foundation" into two decision points is this ADR's most important self-constraint.**

**6. Bitemporal and `supersedes` version anchoring: neither in v1.** No `valid_from/valid_to` columns on `wiki_pages` (no consumer); no attribute columns on `page_links` to anchor version numbers (honoring lane 1's "edge types only for now" promise — adding edge attributes is another rebuild); `lintRelationShape`'s `updated_at` inversion check stays as is. The temporal debt is on the books, to be collected when the first real consumer appears (most likely "replay evidence as of a point in time," or fact pruning during ADR-0019 merges).

**7. No new gate key needed.** Version history is purely local audit data, zero egress, and failure does not affect the main path (the trigger lives inside the transaction; if the page write fails, nothing is left behind). But the "rollback" button must stay in an explicit human action — no automatic path (lint ticker, auto_import, MCP) touches it.

**Explicitly NOT**: bitemporal validity intervals, event sourcing (making `wiki_pages` append-only — every read path would be rewritten), automatic application of revisions, attribute columns on `supersedes` edges, lint staleness checks (neighbors all evolving while one page sits still — a separate low-priority lint item), optimistic locking / conflict merging (v1 covers it with a UI prompt; see open question 6).

## Candidate matrix

| Option | Distance from today | Value | Main risk |
|---|---|---|---|
| **This decision: `page_versions` mirrors `note_versions`** | Near, one v23 | Pages get "a wrong edit you can go back from" for the first time; diff/UI/discipline all exist; the only safe foundation for the revision loop | Storage amplification from full snapshots (page bodies are capped at ≤1500B, tolerable) |
| Full bitemporal (valid_from/valid_to + supersedes anchored to versions) | Far | Can answer "valid as of when" | No consumer; doubles the write path; adding edge attributes means another rebuild |
| Event sourcing (append-only pages + projected reads) | Far | The purest temporal model | Every read path rewritten; rollback semantics redefined; scale overkill |
| Do nothing; keep `compile-revision` todos for manual editing | 0 | Saves a migration | `UpdateWikiPage` never gains rollback; "the compiler revises existing conclusions" stays off the table; the `supersedes` temporal debt keeps accruing |

## Consequences

**Positive**: the page side gains an audit/rollback foundation on par with notes (one mental model for users); ADR-0015's red line gets a concrete future relaxation path instead of a permanent freeze; the version list makes "what changed on this page" visible in the review surface, indirectly restraining careless manual edits; `supersedes`/`contradicts` gain a clear evolution direction for their temporal semantics.

**Negative (unvarnished)**:
- Full snapshots amplify storage — up to 50 bodies per page. Page bodies are capped at ≤1500B by the compiler's budget, so the magnitude is controllable, but on a large library those are real bytes.
- The trigger interacts with lane 2's `page_embeddings` dirty queue: rollback goes through the same `UpdateWikiPage` write entry, and dirtying must happen as usual, or the vector index points at stale text after a rollback. Verify at migration time; if lane 2 lands first, a dedicated regression is required (promotion criterion 5).
- "Version history" is another frontend surface (list + diff + restore confirmation) — copyable from the notes side, but still real work.
- Rollback can collide with concurrent editing (person A is editing while person B rolls back to an old version): v1 ships no optimistic lock, covering it only with a "rollback will overwrite the current content, which will be stored as a new version" confirmation — overwriting never silently loses data, but conflict prompting is a v2 matter.
- Building the foundation does not mean the loop is closed: revision v2 (machine drafts → human applies) remains a separate decision; this ADR deliberately does not decide it for them.

## Open questions (with recommendations)

| # | Question | Recommendation |
|---|------|------|
| 1 | Schema number: v22 is already taken by lane 2's page vectors (frozen) | **This ADR takes v23**, leaving 22 for page vectors; if lane 2 is permanently abandoned, revisit renaming 23 to 22 (not after the migration has shipped). [ADR-0019](0019-entity-resolution-merge.md) decision 6's optional `page_merges` was also penciled in as v23: the tie is broken by **first migration to ship claims the number** — if that optional table ships first, it takes v23 and this ADR slips to v24; if this ADR ships first, it slips to the next free number. Neither side treats the number as a promise; at ship time the `ExpectedSchemaVersion` constant is the source of truth |
| 2 | Version granularity | **Full snapshots** — the notes precedent + ready-made LCS diff; delta storage buys nothing at page scale |
| 3 | Retention | **50 per page**, word-for-word identical to `note_versions`; no configurable key until a real "not enough" signal appears |
| 4 | Snapshot trigger surface | **title/content/kind** (per the v11 lesson); status/source/wall-clock do not snapshot |
| 5 | Bitemporal validity intervals | **Not in v1**; wait for the first consumer; `supersedes` version anchoring queues behind it |
| 6 | Rollback concurrency semantics | **No optimistic lock yet**: the confirmation states "the current content will be stored as a new version"; locking is v2 |
| 7 | Relationship to lint staleness checks | **v1 leaves `lintRelationShape` untouched**; staleness (long-unchanged pages) becomes a separate low-priority lint item, not folded into this ADR |

## Promotion criteria (Proposed → Accepted)

1. Reversible migration: `sqlite_master` snapshot + version multiset reconciliation before/after rebuild, with legal content never rewritten; idempotent replay (lane 1's discipline).
2. Trigger precision regression: any real change to title/content/kind must snapshot; approve/reject/pin-type status transitions **must not** (pinned by the v11 precedent).
3. Rollback never deletes versions: a rollback snapshots the current content as a new version before applying the old one; rolling back to the same version twice yields the same result (idempotent).
4. Deleting a page cascades its versions (FK CASCADE); the `reject≠delete` red line is unaffected (rejecting a pending page still leaves no version residue).
5. No conflict with the `page_embeddings` dirty queue: rollback goes through the same `UpdateWikiPage` write entry and dirties as usual (if lane 2 has landed, a dedicated regression).
6. Rehearse once on a real-DB copy: follow `cmd/wiki-rehearse`'s read-only-copy discipline and confirm the migration loses no content on existing pages.
7. Bilingual docs (a page-side section under "Version history" in `docs/features/knowledge.md`) + this ADR's status line updated; CHANGELOG only when code lands.
