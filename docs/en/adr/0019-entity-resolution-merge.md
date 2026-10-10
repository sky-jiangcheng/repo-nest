# ADR-0019: Entity resolution and concept merging — fixing "concept splitting", where merging is always human-triggered

- Status: Proposed (**design only, no code**. The merge execution + structural candidate discovery can start once ADR-0018 lane 1 is Accepted; the "vector similarity" branch of the blocking depends on lane 2's `page_embeddings`, and when lane 2 has not landed that signal is simply absent — it does not block the rest. See promotion criteria at the end.)
- Date: 2026-10-10
- Relates to: [ADR-0014](0014-llm-wiki-knowledge-compiler.md) (decision 4 "the compiler only creates"), [ADR-0015](0015-w3-minimal-compile-loop.md) (decision 3 explicitly books "concept splitting → a human merges" as an accepted cost; `reject≠delete`; the compiler never edits an approved page), [ADR-0016](0016-async-batch-jobs.md) (if judging is slow it rides the lint job kernel, not a new kind), [ADR-0018](0018-knowledge-graph-foundation.md) (lane 1's controlled relation vocabulary is the edges a merge repoints; lane 2's page vectors are this ADR's optional signal); TODO **M7** first follow-on; starting schema v21
- Companion: Phase 3 of `RepoNest-知识图谱实施计划-2026-10-09.md`. (Chinese original: `docs/adr/0019-*.md`.)

## Context

ADR-0015 decision 3 made an explicit trade: **the compiler only creates pending pages and never edits an approved one.** It stated the cost honestly — "concepts will split (two pending pages for the same topic), and a human has to merge them at review." The problem: **that merge debt has come due, and there is no way to pay it.**

1. **No merge primitive exists.** The review UI (`web/src/pages/Review.tsx`) has only `approve` / `reject` / `delete` (:117-121); a repo-wide grep for `Merge` hits only `MergeProjectUp` (project merge, unrelated) plus path/tag/todo dedupe — **nothing anywhere fuses two pages into one**. ADR-0015 asked the human to hold the bag; the system never gave them a tool.
2. **Lane 1's real run hit exactly this.** Five notes compiled into 7 pending pages; a page-by-page fidelity audit found 4/7 faithful, 3/7 with local hallucinations; "elevating a quick-capture FAB test blip into a concept page" is precisely a split-plus-mislabel only a human can resolve. By that sample, roughly 40% of produced pages get rejected — **rejection leaves a pile of semantically-overlapping fragments, and the graph gets dirtier over time.**
3. **Prevention is incomplete.** The compiler prompt already sends the "existing slug vocabulary" (`wiki_compile.go:213-220` feeds the model the full `ListWikiPages` slug/title/kind/status), yet the model still invents `payment-gateway` and `gateway-payment`. Prompting alone can't stop it; it needs **detection + merge** as a backstop.

## References (borrow the mechanism, not the automation)

- **Classic entity resolution / record linkage**: `blocking` (cheap candidate finding, cutting O(n²) to a feasible set) → `matching` (decide equality) → `merge`. The engineering crux is **blocking**, not asking the model about every pair.
- **Microsoft GraphRAG entity resolution**: uses embeddings + textual evidence to let the LLM decide "same entity?", then merges nodes — but it **lets the model merge automatically**. This repo borrows **only the equality decision, not the automatic merge**: merging is information destruction, so it stays behind a human.
- Karpathy / nashsu do no automatic merge either.

## Decision

**1. A merge is a state transition only a human can trigger** (inheriting ADR-0015). No automatic path — compiler, model, lint, or job — may call merge; only the review UI, at a human's click. This is the criterion most needing approval, and the fundamental divergence from GraphRAG.

**2. Candidate discovery is purely structural — no model, no vectors — and can run now.** Blocking uses four cheap signals (none depend on lane 2):
- **slug shape**: after `NormalizeWikiSlug`, one is a prefix of the other or edit distance is small;
- **shared source notes**: their `note_pages` note-id sets overlap (same source, likely same concept);
- **title-token overlap**: high Jaccard on tokens;
- **already cross-linked but duplicative**: a `ref`/`mentions` edge between them yet each restates the thing (this is the first place lane 1's relation edges are **read**).

This step **produces candidate pairs only, never conclusions**, and uses SQL/in-memory to cut candidates from n² to top-K.

**3. Equality in two tiers; vectors optional.** Tier one is the cheap score; only pairs over a threshold go to tier two, which **optionally** adds cosine similarity over lane 2's `page_embeddings` — when lane 2 hasn't landed that signal is 0 and **does not block** decisions 2/4/5. So this ADR stands on its own before lane 2.

**4. The model only decides equality; it never writes to the store.** For a candidate pair, one call: "are these the same concept? If so, which to keep as winner, and why?" Output `{"same":bool,"loser":"slug","winner":"slug","reason":"…"}`. New key `wiki_merge_suggest` (default off, reuses `ai_chat_*`, capped pairs/run e.g. ≤200). The result **becomes only a `[lint] merge <loser> → <winner>` todo** — reusing `fileFindings`' write-only-to-`project_todos` discipline (`wiki_lint.go`); **the model cannot merge.**

**5. Executing a merge is one transaction, only on a human's accept:**
- repoint every incoming/outgoing edge of the loser to the winner (`LinkWikiPages(…, 'ref')` is idempotent; if the winner already has the edge, `UNIQUE` drops it);
- move the loser's `note_pages` source attachments to the winner (provenance preserved);
- set the loser to `status=rejected` + replace its body with "merged into [[winner]]" — **`reject≠delete`, keep the row** (deleting would cascade away edges not yet moved and destroy the audit trail); because edges were all repointed, the winner absorbs every reference and the loser leaves no dangling inbound link;
- the winner's `id/slug/content/updated_at` stay **byte-for-byte unchanged** — a merge is "the loser is absorbed", not "edit the winner", so it still honors ADR-0015's "do not modify approved page content." A human changes edges and the loser, never the winner's body.

**6. Optional audit table `page_merges(loser_id, winner_id, kept_at)` (v23 if needed).** A plain `CREATE TABLE` that **touches no CHECK family** (avoiding the "add a value = rebuild the whole table" trap of `compile_jobs.kind` / `wiki_pages`). Used for a merge history and a future "undo a merge" (restore the loser from rejected, repoint edges back). **Off by default**; add it only when a real undo need appears — continuing ADR-0018's restraint of "explicit replacement, never silent merge."

**7. Prevention tightening (push the debt earlier).** Add a hard rule to the compiler prompt: "if this note is about a concept already in the vocabulary, use `add_link`/`attach_note` to point at it — do not `create_page` a near-duplicate." Prompt-only, low risk, reduces the rate at which candidate pairs are born.

## Candidate matrix

| Option | Value | Main risk |
|---|---|---|
| Fully automatic merge (GraphRAG-style) | Hands-off | Merging is **information destruction** — harder to spot than splitting (redundancy) and **irreversible**; letting a model destroy human-approved knowledge is exactly what ADR-0014/0015 refused |
| Exact slug match only | Free | Misses `payment-gateway` vs `gateway-payment` word-order/synonym splits |
| No blocking, ask the model about every pair | Highest recall | O(n²) LLM calls, token cost runs away (ADR-0015 already warned cost grows super-linearly with page count) |
| Wait for lane 2 vectors before any disambiguation | Richest signal | Chains usable structural disambiguation (decisions 2/5) to vectors that don't exist yet — throws the baby out |
| **This decision: structural blocking + cheap equality + optional vector enhancement + human merge** | Buildable now, cost-bounded, destruction held by a human | Bad thresholds miss or spam; equality judging still errs |

## Consequences

**Positive**: gives ADR-0015's "accepted cost" a real repayment tool; near-duplicate pages can be consolidated and the graph stays clean; lane 1's relation edges get read for the first time (candidate discovery uses them); merges are human-in-the-loop, so destruction is controllable and auditable; structural blocking does not depend on the un-landed vectors, so it can proceed independently.

**Negative (no sugarcoating)**:
- Merging is still **destructive**; wrongly fusing two genuinely different pages loses knowledge. Three mitigations (winner body untouched / audit table / human in the loop) blunt it but **do not eliminate** it.
- Candidate thresholds are a new tuning surface: too loose → every pair becomes a suggestion → review fatigue (the failure mode Lane 1 already surfaced); too tight → splits persist.
- Equality judging still costs money and is still fallible, so it stops at "a suggestion for a human," not "a decision made for you."
- The cosine signal that relies on lane 2 is absent until lane 2 lands, so early disambiguation precision is reduced (but no function is missing — see decision 3).
- This is another three-layer change (db merge transaction + service blocking/judging + web merge-suggestion area), not a single-file edit.

## Open questions (with recommendation)

| # | Question | Recommendation |
|---|----------|----------------|
| 1 | Who picks the winner | The model suggests one (more sources / earlier / higher in-degree), the UI lets a human flip it; the human's click is final |
| 2 | Can a merge be undone | For MVP keep only a deletable `rejected` tombstone; real undo (decision 6's `page_merges`) comes with an actual need |
| 3 | Where judging runs | Fold into the lint model side (reuse `wiki_lint_llm` + job + budget), **do not add a job kind** (honor ADR-0016's one-table-two-kernels and avoid rebuilding `compile_jobs.kind`); but gate it with its own key `wiki_merge_suggest`, separate from contradiction/stale |
| 4 | Cross-project merges | Not in MVP: require both pages in one project, or at least one global (`project_id=0`), or repointing edges would silently strip a loser-project owner's knowledge |

## Promotion criteria (Proposed → Accepted)

1. **Lane 1 is Accepted** (v21 real-DB rehearsal passed) — a merge repoints vocabulary-constrained edges, so it sits after lane 1.
2. Merge-transaction contract tests: (a) winner content/updated_at byte-identical; (b) after merge the loser has zero in/out edges and `status=rejected`; (c) `note_pages` sources moved to the winner, none lost; (d) replaying the same pair is idempotent (edges absorbed by `UNIQUE`).
3. Candidate-discovery tests: each of the four signals gets one positive and one should-not-fire negative; top-K truncation works; assert it does not do a full n² sweep.
4. Model side: non-JSON / a fabricated winner/loser slug → trust zero suggestions (the same rule as lint: a page_id/slug must resolve in the store to be believed); `wiki_merge_suggest` off means zero outbound calls.
5. The review UI has a merge-suggestions section with accept/ignore, exercised **end-to-end once on a real-DB copy** — not fixture-only (Lane 1's gate lesson: mechanism correctness ≠ real gain).
