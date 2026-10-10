# ADR-0020: Community summaries — let the synthesis layer grow on its own via Label Propagation

- Status: Proposed (**design only, no code.** Promotion prerequisite: ADR-0018 lane 1 Accepted — community detection propagates along the controlled relation whitelist, so the vocabulary is its foundation. The trigger model (synchronous vs job) in decision 6 / open question 4 is the item most in need of a decision here.)
- Date: 2026-10-10
- Relates to: [ADR-0014](0014-llm-wiki-knowledge-compiler.md) (decision 1 lists synthesis among the five kinds; decision 5's lint carries the `no-synthesis` gap check), [ADR-0015](0015-w3-minimal-compile-loop.md) ("MVP does not auto-rewrite synthesis"; only create pending, never edit an approved page; review isolation), [ADR-0016](0016-async-batch-jobs.md) (async jobs, but adding a value to `compile_jobs.kind` = full table rebuild), [ADR-0018](0018-knowledge-graph-foundation.md) (lane 1's vocabulary = the edges this ADR propagates along; lane 2's PPR was "reading the graph the first time", this is the second), [ADR-0019](0019-entity-resolution-merge.md) (merging makes communities cleaner but this ADR does not block on it: split concepts can still be summarized, only at reduced quality); TODO **M7** follow-on; starting schema v21
- Companion: Phase 4a of `RepoNest-知识图谱实施计划-2026-10-09.md`. (Chinese original: `docs/adr/0020-*.md`.)

## Context

ADR-0014 decision 1 gave the page layer five kinds: `entity / concept / source / synthesis / query`. But **only four got a generation side**:

1. `entity/concept/source/query` — the compiler produces them per note (W3, pending → human review), and answer-filing produces `query` (W2). **`synthesis` alone has no systematic generation mechanism.** The compiler prompt lets the model occasionally label one page `synthesis` (`wiki_compile.go:225` includes it in the kind enum), but that is an ad-hoc label from a single-note viewpoint, not "synthesize a topic into one point of view across sources" — synthesis means a review, and the compiler reads one note at a time, so it structurally cannot produce one.
2. So W4's `no-synthesis` check (`wiki_lint.go:271`: ≥5 leaf pages yet zero synthesis) **is always red** — it honestly reports "knowledge is still scattered fragments," but nothing in the system can turn it green. This is a **dead loop of an anti-corruption layer pointed at an idle generation layer**: lint cries "missing synthesis," and nobody can fill it, because there is no step that generates synthesis at all.

ADR-0015 said "MVP does not auto-rewrite synthesis"; ADR-0018 pushed it to "each its own ADR." It is exactly the hole that was twice explicitly deferred and never filled. This ADR fills it.

## References (copy the mechanism, not the stack)

- **Microsoft GraphRAG global search**: community-detect the graph (Leiden) → have an LLM summarize each community → synthesize upward layer by layer. This is the standard "grow points of view across sources." But its stack binds Leiden multi-layer aggregation + map-reduce global summaries; RepoNest's local-first / zero-dependency / single-SQLite cannot and need not use it (a few hundred to a few thousand pages). **Borrow only the "community → summary" mechanism.**
- **Label Propagation (LP)**: Raghavan 2007 classic community detection — each node adopts the majority label among its neighbors over a few rounds. Pure Go, dozens of lines, unweighted, no external library. Lower quality than Leiden, but enough at this scale, and **simple = auditable** (fitting this repo's reversibility/lint ethos).
- Karpathy / nashsu: synthesis is human-written, the tool does not generate it. This repo splits the difference — **the machine drafts to pending, a human approves it into being** — closing the generation gap without bypassing the human.

## Decision

**1. Use Label Propagation, not Louvain/Leiden.** LP is dozens of pure-Go lines, zero dependency, zero CGO. Leiden has better community quality, but its multi-layer aggregation + modularity-optimization code and tuning surface is over-engineering at a few-hundred-page scale and harder to audit. **Explicitly accept LP's two known weaknesses** (blurry boundaries, occasional collapse into a giant blob), bounded by size thresholds (decision 2).

**2. Community-size thresholds.** Discard fragments `< minCommunitySize` (default 3) — they are not a "topic" and summarizing them is forced; cap blobs `> maxCommunitySize` (default 30) — an LP-degeneracy product that also busts the model context budget. Both configurable.

**3. Propagate only along the controlled relation whitelist.** Walk `ref / part-of / depends / implements / documents` (signals of "about the same thing"); **not `contradicts`** (opposition isn't same-topic), **not `mentions`** (a passing body link does not imply same community). This is the first time ADR-0018 lane 1's vocabulary is **fed into an algorithm** — with free-text relations, a whitelist could not even be defined. Tie-break: initial labels by member-slug lexicographic order, ties resolved to the lexicographically smallest neighbor label — **guaranteeing the same graph + input yields the same partition** (LP's intrinsic randomness is pinned down by a deterministic tie-break, which is what makes the idempotency test hold).

**4. Input is approved pages only; output is pending only.** pending/rejected never enter detection (a quarantine zone, not part of "what the current knowledge looks like"). Each community yields one `synthesis` page with `status=pending`, `source='wiki-community'` (distinct from the compiler's `'wiki-compile'`, so the review UI sees provenance at a glance). Full ADR-0015 discipline: create only, never edit approved, human approval required to enter search.

**5. Synthesis page ↔ members are linked by `documents` edges.** Each member page gets a `documents` edge to its synthesis page (the vocabulary's proper use of "describes": a review describes the pages it covers). Edges are built by **code**; the model gets no `add_link` power. Synthesis body ≤1500B (reusing the compiler's per-page byte budget).

**6. The model only writes the body, no structural power.** Show the model: member-page excerpts (each truncated, total budget-capped) + the whitelist edges between them. It outputs **a single plain-text review** (not op JSON); the page, edges, and source attachments are all done by code. This is **less power than the W3 compiler** — the compiler can at least choose kind/slug; the community-summary model only fills a body and cannot invent a page or an edge.

**7. Idempotent + never rewrite an approved synthesis.** Derive a stable synthesis slug: `synthesis-` + first 8 hex of sha256(the sorted member-slug list) — recompute the same community, get the same slug. Collide with an existing `pending` (this loop's own prior output) → update; collide with an **approved** synthesis → **do not edit**, downgrade to a `synthesis-revision` todo (exactly reusing the compiler's `fileRevisionTodo`). Honors ADR-0015.

**8. Trigger: synchronous run with hard budgets; no job in MVP.** Per-run caps `≤10 communities / ≤24 pages each / ≤1500B each / total wall-clock ≤DefaultBatchChatTimeout`, stop on overrun and say where it stopped in the report (reusing ADR-0015/0016's "budget = full stop"). **Why not a job yet**: adding `'community'` to `compile_jobs.kind` needs a full table rebuild (SQLite CHECK can't be ALTERed — a hard constraint), and paying that rebuild for a manual, low-volume action is not worth it; when a real "whole-corpus summary takes many minutes / needs a queue" signal appears, do it together with the rebuild ADR-0019 will eventually need. **This is open question 4, the item most needing your decision.**

**9. Gate `wiki_community_summary` (default off), never in a ticker.** Lint can run on a 6h timer because it is read-only and cheap; a community summary is one LLM call per community that writes pending pages — a **spending generation action** that must be a human's explicit click. Not in `startWikiLintTicker`, not in `auto_import`, not an MCP tool (same reasoning as ADR-0015 decision 6: an agent able to pour reviews into the store = bypassing the human).

**Explicitly not doing**: Leiden multi-layer summaries (over-engineering), synthesizing synthesis pages into a higher layer (get single-layer working first), job-ifying it (open 4), cross-project communities (MVP detects per project; global pages are their own bucket), auto-merging revisions into synthesis (a todo instead, decision 7).

## Candidate matrix

| Option | Distance | Value | Main risk |
|---|---|---|---|
| Copy full GraphRAG (Leiden + layers + map-reduce global) | Far | Industrial-grade topic summaries | Breaks local-first / zero-dep; unnecessary at this scale; un-auditable |
| Louvain single layer | Medium | Better communities than LP | Modularity code + tuning far larger than LP, marginal gain here |
| **This decision: LP + relation whitelist + pending output + human review + sync budgets** | Medium | Closes the synthesis generation side, greens no-synthesis, reads the graph a 2nd time | LP boundaries imperfect; patchwork risk |
| Do nothing, synthesis always by hand | 0 | Saves effort | `no-synthesis` is a permanent-red dead loop; four-of-five kinds is a broken set |
| Skip the graph, just "summarize the whole store" | Looks simpler | One shot | Wandering topic boundaries, no community structure, busts context, output not idempotent/auditable |

## Consequences

**Positive**: all five kinds finally have a generation path for the first time; `no-synthesis` goes from "crying" to "fillable green"; this is the **second real read of the graph** after ADR-0018 lane 2 (using the vocabulary as algorithm input, not just a lint criterion); the model has less power than the compiler (body only); output defaults to pending, so isolation/review/rollback infrastructure is entirely already-built.

**Negative (no sugarcoating)**:
- LP quality depends on graph structure; boundaries blur and blobs happen — **it promises not "correct" communities but "idempotent + size-bounded."**
- A hallucinated summary written into a `synthesis` page **looks more authoritative** than a single compiled page (its kind literally says "review") — the error is subtler. The only barrier is the human in the loop + excerpt budget + the `wiki-community` provenance tag that warns a reviewer.
- When a community lumps unrelated pages, the summary becomes a patchwork; current lint checks single pages/edges, not whether a set is coherent. Recommended at code time: add a `community-incoherent` check (via `wiki_lint_llm`, low-confidence, todo-only) — but this ADR does not block on it.
- Sync-vs-job is a real open item (decision 8 / open 4); synchronous is fine on small stores but a few-hundred-community store would hit the timeout.
- Another three-layer change (new algorithm package + service generation + trigger binding/UI), not a single-file edit.

## Open questions (with recommendation)

| # | Question | Recommendation |
|---|----------|----------------|
| 1 | Where the detection lives | New `internal/graph/lpa.go` (pure function, graph in / communities out, zero db dependency, independently unit-tested); the service layer only "reads edges → feeds lpa → writes pages" |
| 2 | LP rounds / tie-break | Default ≤15 rounds; slug-lexicographic tie-break for idempotency; if it fails to converge, take the current state |
| 3 | Synthesis provenance tag | `source='wiki-community'` (a new constant), not reusing `wiki-compile`, so review and lint can distinguish origin |
| 4 | **Trigger: sync+budgets vs job** | **MVP sync + hard budgets**; tie "should we job-ify it" to the same `compile_jobs` table rebuild ADR-0019 will eventually need, and do them together when a real long-duration signal appears |
| 5 | Are the granularity thresholds configurable | min=3/max=30 as code defaults, exposed as two overridable numeric config keys (via `allowedConfigKeys`) |
| 6 | Global pages (`project_id=0`) | Run a separate detection pass, never mixed into a project; their synthesis lands at `project_id=0` (cross-project review), which happens to feed W5's L3 |

## Promotion criteria (Proposed → Accepted)

1. `internal/graph/lpa` pure tests: chain / tree / two-communities-one-bridge / isolated triangle / giant blob — expected partitions correct, min/max thresholds apply, **re-running the same graph yields a byte-identical partition** (tie-break idempotency).
2. Generation contract tests (`service` layer, mirroring `wiki_compile_test`): a non-text/empty reply → the body is not written but the page, `documents` edges, and source attachments still land; re-run is idempotent (same slug, no second page); **colliding with an approved synthesis does not rewrite it, it downgrades to a `synthesis-revision` todo**; hitting the budget stops and is reported.
3. `wiki_community_summary` off means zero outbound calls and zero writes; on means the report carries the outbound byte count.
4. End-to-end rehearsal on a real-DB copy once, output reviewed by a human (carrying W3's gate lesson: mechanism correctness ≠ real gain).
5. After code lands, the `no-synthesis` lint check must be able to turn green after "ran community summaries and approved some synthesis" — using its own check to prove the generation side works.
