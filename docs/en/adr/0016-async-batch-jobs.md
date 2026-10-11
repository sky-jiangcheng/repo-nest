# ADR-0016: Async long-running batch jobs — submit returns, progress is queryable, results survive

- Status: Accepted (the async job and a standalone Review tab are both implemented: approve / mark-unqualified / delete-permanently for pending pages, plus job submission and polled progress in the UI. **Open question ① landed**: model-backed lint shares the same job table — schema v20 adds a `kind` discriminator plus a `findings` counter, one queue with two kernels. **Open questions ② ③ remain decisions**: a crashed `running` job is still marked failed rather than re-queued, single worker unchanged. Interactive Q&A is **intentionally synchronous** — it has exactly one waiter, so turning it into a job would only add a polling loop without shortening the wait by one second; what actually needed fixing was its timeout ceiling, raised from 90 seconds to 5 minutes, see the revision below).
- Date: 2026-10-08
- Relates to: [ADR-0015](0015-w3-minimal-compile-loop.md) (compilation), [ADR-0014](0014-llm-wiki-knowledge-compiler.md) (W3/W5), TODO M6

## Context

The real-library rehearsal did not question compilation's correctness; it questioned its **transport shape**. A single `CompileProjectNotes` took 173 seconds on a local 27B model for one note; five notes mean tens of minutes. Synchronous RPC is wrong at that scale for two reasons:

1. When the client times out or disconnects, **the server's work has already committed**. That is exactly what the old 60s write deadline in `cmd/server` produced: curl reported `http=000` while the database gained three pending pages. Reporting a successful operation as a failure is the class of bug that teaches users to stop trusting the tool. A stopgap now keeps the write deadline above the batch ceiling (with a startup drift assertion), but that only makes a synchronous call less likely to break — it does not make it sane.
2. Synchronous means **the UI must hang** waiting: no progress, no cancellation, and navigating away loses the result. Batch compilation is precisely the interaction that needs "I can leave, and see the outcome later".

## Decision

### 1. A job table with submit/poll semantics; the compiler itself is untouched

`compile_jobs(id, project_id, requested_notes, status, notes_done, notes_total, pages_created, pages_updated, links_created, attachments, revision_todos, rejected_ops, stopped, llm_note, error, created_at, started_at, finished_at)`, `status IN ('queued','running','succeeded','failed','canceled')`.

The compile core in `wiki_compile.go` stays as it is: a pure "given one note, produce ops and persist them" unit. Async is only a loop around it. **No second implementation of compilation.**

### 2. Reuse the existing background-goroutine discipline; no new scheduler

Jobs are consumed by a single worker started with `bgGo` (one job at a time, so it cannot fight SQLite's single writer), and `Shutdown` already waits for it. On restart, jobs stuck in `running` are re-queued or marked `failed` by `started_at` — an explicit failure beats pretending to resume, since we do not know where it stopped.

### 3. Cancellation means "stop before the next note", never interrupting an in-flight request

Interrupting a waiting LLM call would mean threading a context through the HTTP client layer to save one already-spent request, at the cost of cancellation semantics everywhere. `canceled` stops the loop once the current note finishes.

### 4. The UI polls only while a job is alive

`GetCompileJob(jobID)` / `ListCompileJobs(projectID)`; the panel shows `notes_done/notes_total` plus the result summary and refreshes the review-queue badge on completion. No WebSocket: the repo's event channel needs extra plumbing in browser mode, and a few-seconds poll costs nothing on a single-user desktop.

## Consequences

Positive: long jobs stop depending on timeout numbers accidentally lining up; progress and cancellation become first-class states; desktop, browser and `/api/rpc` all read the same job record. Negative: one more table and state machine, and "the button returned without an answer" must be expressed clearly in the UI or it reads as a hang — arguably a bigger copywriting risk than the current one. Async also does not shrink the real cost (2-3 minutes per note locally); it makes the cost visible.

## Open questions

1. Should the same table carry model-backed lint (equally long)? → **Decided: yes** (schema v20, see the revision below): one job table, one polling surface.
2. Crash handling for `running` jobs: re-queue or mark failed?
3. Do we ever need concurrent jobs (default single worker unless users compile several projects at once)?

> **Post-implementation revision (same day)**: acceptance ②'s "disconnect drill" **changed object** in the async shape — killing the client no longer matters (submission returns in milliseconds; the work is already in the database). What the tests pin instead are two things: when the worker sees `ctx.Done()` (app exit) the job lands in `canceled`, and on restart a stuck `running` job is explicitly marked `failed` and never re-claimed.
> One more rule the ADR text never wrote but would have bitten: **a late progress write must not resurrect an already-canceled job** (`WHERE ... AND status <> 'canceled'`).
> The compile core really was reused with zero changes — the job loop calls the existing single-note compile entry, so acceptance ① holds.
> The synchronous path stays, and shares the **same "which notes" function** with the job path (`compileNoteTargets`, changed to newest-first); otherwise the same "compile this project" would cover different notes on the two paths.

> **Open question ① landed (same day, schema v20)**: `compile_jobs` gains `kind` (`'compile'` / `'lint'`, CHECK-guarded, defaulting to `'compile'` — pre-existing rows were compile jobs) plus a `findings` counter; the migration `EnsureCompileJobQueue` uses ALTER rather than a new table, so a user's compile history survives. `runNextCompileJob` dispatches on `kind` to the two existing synchronous entries (`CompileNote` / `RunWikiLint`), **with no second implementation** — that is exactly this ADR's boundary test.
> Two decisions worth recording that differ from the original sketch:
> - **A lint job must carry a project** (`db.CreateLintJob` rejects project=0). `project_id` is `NOT NULL` with a foreign key to `projects(id)`, and the "all pages" scope has **no representation in the database**; inventing a sentinel project would pollute the user's project list. So scheduled full-corpus lint still takes its original synchronous path; what enters the queue is the panel-triggered per-project model check.
> - **The structural checks did not join the job table**. They are pure SQL, millisecond-scale; making a user poll a job row for them is theatre, not engineering. Only the two model-backed checks — the ones that can run for minutes — enter the queue.

> **Why interactive Q&A is not async**: this one contradicts the ADR's original assumption, and the reason is payload, not preference. The evidence path sends layered context (L3 1200 + L2 1600 chars) plus up to 12 retrieved items (~4000 chars) — the same order of magnitude as one compile request, and compilation measured 173 seconds on a local 27B. Catching that with the 90-second `aiChatTimeout` would report "the model is still thinking" as "failed" — the same class of error as the original too-short batch ceiling, in the other direction. So the thing to fix was the ceiling: `aiAskTimeout = 5 minutes` (no larger than `DefaultBatchChatTimeout`, since the HTTP write deadline derives from the latter). Synchronous stays: there is exactly one caller and the panel already has a pending state; making it a job would only add a polling loop while the wait stays identical. Streaming — which would change the wait itself — remains outside this ADR.

## Promotion criteria (Proposed -> Accepted)

1. The compile core is reused with zero changes; if a second implementation appears, this ADR drew the boundary wrong.
2. Disconnect drill: kill the client mid-job; job record and produced pages remain complete and the status converges correctly.
3. Cancellation drill: after `canceled`, no new note is processed and no half-written note leaves orphan artifacts.
4. The crash-recovery path is tested, not merely described in a comment.
