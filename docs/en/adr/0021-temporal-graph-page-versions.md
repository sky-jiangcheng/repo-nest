# ADR-0021: Temporal graph — `page_versions` snapshots make safely revising approved conclusions possible

- Status: Proposed (**design only, no code**. Prerequisites: ADR-0018 lane 1 Accepted; and it is only worth landing when the first real "an approved page is stale and needs revision" need appears — implementing earlier means paying maintenance for a mechanism with no user. Trigger timing: pending call 4)
- Date: 2026-10-12
- Related: [ADR-0014](0014-llm-wiki-knowledge-compiler.md) (the SSOT lives in the DB, not the export tree — snapshots must live there too), [ADR-0015](0015-w3-minimal-compile-loop.md) ("never modify approved pages" — this ADR is its first controlled loosening channel and does not touch that invariant itself), [ADR-0016](0016-async-batch-jobs.md) (adding a `compile_jobs.kind` value = full table rebuild, a red line), [ADR-0018](0018-knowledge-graph-foundation.md) (lane 1's `supersedes` time-ordering lint is the first consumer of temporal semantics, yet it only has the single mutable `updated_at` to go on), [ADR-0019](0019-entity-resolution-merge.md) / [ADR-0020](0020-community-synthesis.md) (precedent for batching into the same CHECK table rebuild); TODO **M7** follow-ups; starting schema v21
- Precedent: the note layer already has `note_versions` (P19 version-history diff) — this brings the page layer up to the same audit capability

## Background

ADR-0015 fixed a hard rule: the compiler **never modifies an approved page** — on a same-slug collision it degrades into a `compile-revision` todo (`internal/service/wiki_compile.go:423`), and revision is entirely manual. That invariant is perfect while nobody proposes revisions, but it also means:

1. **There is no auditable path to revision.** Knowledge goes stale — a note says the API moved to v2 while the page still concludes v1. The only ways out today are "write another page", which is exactly the "concept splitting" ADR-0019 treats, or hand-editing the page — and `wiki_pages` is a mutable row whose `content` is overwritten in place: **once changed, you can no longer see what was approved at approval time**.
2. **`supersedes` temporal semantics have no data to stand on.** Lane 1's lint can catch "supersedes with inverted time order", but which of two pages is older rests on a single mutable `updated_at`, lost the moment the page changes.
3. **Notes have version history (`note_versions` + P19 diff); pages do not.** AI-generated content needs "approval-time reproducibility" more than handwritten notes do.

"Safely letting the compiler revise existing conclusions" is an explicitly parked M7 direction, and its prerequisite is: **snapshot before every change**. This ADR fixes only the shape of that foundation; it lands no code.

## Prior art (copy the mechanism, not the stack)

- **Our own `note_versions`**: append-only version rows + the live current row + a diff view. The page layer copies this proven pattern instead of inventing a new one.
- **MediaWiki revisions**: one revision per change, "current" is a query for the latest. **Only the "append-only + look-back" mechanism is borrowed, not the architecture** — making "current = query for latest revision" would turn FTS, export, evidence and vectors into join reads, a disproportionate cost (see the matrix).
- **The transaction time / valid time split from temporal databases**: this repo needs only transaction time (when this version was written/approved), not valid time (when the fact was true) — the latter requires humans to annotate effective times, and this repo has no such input source.

## Decision

**1. New table `page_versions`, snapshotting approved content only; the live page row stays mutable.** The read paths of `wiki_pages` (FTS v17, search, export, evidence, AskAI) get **zero changes** — the current view is always the live row; the snapshot table is a pure audit layer. Minimal columns: `id / page_id / title / content / kind / source / created_at` (a snapshot records "what the page looked like at approval time"). pending and rejected pages **never get snapshots**: they are unreviewed drafts or error placeholders by definition, with no "conclusion history" to audit.

**2. Exactly two write points.** (1) `ApproveWikiPage` freezes a snapshot — "this exact content was approved" becomes reproducible; (2) any future path that modifies approved content freezes the old version before taking effect. The first migration backfills one snapshot per existing approved page (`created_at` taken from the page's `updated_at`, meaning "this content has held since then").

**3. Append-only is enforced by triggers, not by goodwill.** `BEFORE UPDATE/DELETE → RAISE(ABORT)` triggers turn "snapshots are immutable" from discipline into structural fact — the same repo instinct as pinning "lint cannot mutate pages" with `TestWikiLint_NeverMutatesPages`: an invariant that can be structural must not live in a comment.

**4. The revision flow (shaped here, not landed in this ADR): `revise_page` proposes a version for human review.** The compiler gains one op (the contract type goes from 3 ops to 4 — a Go type change, touching no CHECK): the target must be an approved page, and the product is not a mutated page but a proposed version row with `status='pending'` plus a `[compile] revise` todo; the review UI diffs current content vs the proposal, and accepting means "freeze the old snapshot first, then write the live page" while rejecting discards the proposal and leaves the live page byte-for-byte unchanged — the spirit of `reject≠delete` applies verbatim. **No new `compile_jobs.kind` value** (reuse `'compile'`, distinguish at the op level) — no full table rebuild is paid for a capability still pending a need.

**5. Migration number v23, with a preference for batching into the eventually-needed CHECK rebuild.** Lane 2 already claims v22; if `compile_jobs.kind` ultimately gains values (ADR-0020 decision 8's batching instinct) or the wiki layer needs another DDL-level change, the snapshot table rides that rebuild. A standalone v23 is also viable; choosing between them is pending call 1.

**6. Minimal temporal semantics.** Transaction time only; no "graph as of time T" replay (graph queries always see the current edge set); no valid time; no retention cap (keep everything at this repo's scale, see pending call 3).

## Options considered

| Option | Distance from status quo | Value | Main risk |
|---|---|---|---|
| MediaWiki mode (current = query for latest revision) | Far | A single shape of truth | FTS/export/evidence/vectors all become join reads; every existing consumer changes |
| Git-style branch/merge history | Far | Forkable knowledge | Over-engineering; no user input source for "knowledge branches" |
| **This decision: live page + snapshot at approval/before change + human-reviewed proposed versions** | Medium | Auditable revision, reproducible approval time, groundwork for revise_page | Consistency test surface between snapshots and live pages; the table grows linearly with edits |
| Do nothing (keep compile-revision todos + manual edits) | 0 | Cheap | History is lost forever; "write another page" worsens concept splitting |
| Keep copies only in the export tree | Small | Look-back on the export side | The export tree is a read-only derivative and holds no SSOT (ADR-0014 decision 2); the DB still has no history |

## Consequences

**Positive**: "approval time" becomes reproducible for the first time (an anchor for audit and look-back); revision turns from "manual + overwrite" into "propose → diff → human review → old version archived automatically"; the `supersedes` lint gains real temporal data; ADR-0020's `synthesis-revision` and any future auto-rewrite discussion get a landing spot.

**Negative (no varnish)**:
- One extra write per approval (KB-scale, tolerable), and the snapshot table grows monotonically — with no retention policy that means "never cleaned"; accept first, observe later.
- The append-only trigger turns "fix a bit of history data" maintenance into a migration exercise (drop trigger, change, rebuild) — a deliberate price paid for non-repudiation.
- Proposed versions are a new review surface: the review UI needs a "diff two contents" view (P19's NoteDiff rendering can be reused, but it is still new code).
- This is again "foundation without feature": without revise_page, the only write point is approval — the full value is realized only once the revision flow lands.

## Pending calls (with recommendations)

| # | Question | Recommendation |
|---|----------|----------------|
| 1 | Migration shape: standalone v23 vs batched with the `compile_jobs.kind` CHECK rebuild | **Batch** (do it when ADR-0019/0020 or lane 2 produces a real DDL need; until then this ADR stays design-only) |
| 2 | Append-only enforcement | `BEFORE UPDATE/DELETE RAISE(ABORT)` triggers (hard constraint) plus a schema assertion test that the triggers are in place |
| 3 | Retention policy | Keep everything, no cleanup; revisit a cap only if a live-DB rehearsal shows runaway growth |
| 4 | When does `revise_page` land | **No separate project**: wait for the first real "approved page is stale" case, user-triggered; until then `compile-revision` todos carry on |
| 5 | Diff presentation in the review UI | Reuse the P19 NoteDiff rendering path; write no new diff algorithm |
| 6 | Record the actor on snapshots | Not in MVP (single machine, single user; `source` suffices); add a column when a B-side multi-user need appears |

## Promotion conditions (Proposed → Accepted)

1. Migration tests: snapshot table creation, backfill reconciliation (exactly one snapshot per approved page, byte-identical content), idempotent replay, `DropWikiSchema` reversibility (the W1 constructive-proof pattern).
2. Append-only tests: UPDATE / DELETE on `page_versions` must be rejected by the triggers; `ApproveWikiPage` re-entry produces no duplicate snapshot.
3. Contract tests (if revise_page lands): proposed versions are invisible to search / AskAI evidence; after acceptance the old content is recoverable from snapshots; after rejection the live page is byte-for-byte unchanged.
4. One live-DB copy rehearsal (`cmd/wiki-rehearse` discipline: read-only source + disposable copy), backfill reconciliation passing.
