# ADR-0011: Multi-agent memory importers — reusable pipeline + per-source feasibility

- Status: Proposed (framework + six sources landed: claude AUTO; codex / opencode / cursor / openclaw / hermes-curated as opt-in MANUAL. hermes sessions pending schema)
- Date: 2026-10-03
- Relates to: ADR-0002 (in-process plugin runtime), ADR-0007, ADR-0010, TODO M2. (Chinese original: `docs/adr/0011-*.md`.)

> **2026-10-11 anchor audit note**: every code anchor in this record is a pre-decision snapshot as of 2026-10-02 (much changed after the importer framework and sources landed); it stands as a historical record of motivation and is not updated as code drifts.

## Background

RepoNest already has a clean pipeline: a `plugin.KnowledgeImporter` yields `[]plugin.ImportDoc` → registered via `Runtime.RegisterSource` → `upsertDoc` idempotently upserts notes keyed by (project, source, title). Originally only Claude was implemented. M2 extends it to other agents, whose on-disk formats are undocumented and drift.

## Decision

1. **Reuse the pipeline**, no new mechanism — each source is a Go `KnowledgeImporter`.
2. **Shared helpers extracted when the 2nd source landed** (`internal/importers/memsrc`: `MatchProject`, `ReadCapped`, `TargetProject`, `StripFrontmatter`, `ClipToBytes`).
3. **Per-source feasibility, verified against real files (never guessed):**
   - **codex** ✅ `~/.codex/sessions/**/rollout-*.jsonl` (streaming; `cwd`→project).
   - **opencode** ✅ `~/.local/share/opencode/storage/session/<hash>/ses_*.json` (already summarized: title/summary/directory).
   - **openclaw** ✅ global `~/.openclaw-autoclaw/workspace/*.md`.
   - **hermes** (Nous Research, separate product) ✅ curated `~/.hermes/memories/*.md`; sessions (`state.db`) deferred (no documented schema).
   - **cursor** — verified → **deferred**: history is in an unofficial `state.vscdb` (bubble blobs + versioned headers + ProseMirror; no project path in-DB) → low-ROI/fragile.
     > **2026-10-11 audit note**: that deferral was **overturned by real-machine re-verification on 2026-10-09 and cursor shipped** as the 6th built-in source (opt-in MANUAL, best-effort, never runs at startup). The re-verified facts: bubble keys look like `bubbleId:<composer>:<uuid>`, `json_valid=1` plaintext, `type` is a numeric sender enum (1/2 only across 71 real rows), `createdAt` is an ISO-8601 string, and `composerHeaders.workspaceId` is exactly the `workspaceStorage/<32hex>` directory name — so attribution is `workspaceId → workspace.json's folder → project match`. The "whole source errors on an unrecognized table/column shape" constraint answers the original "cannot validate the parser" concern. See CHANGELOG [1.16.0] "6th built-in knowledge source: Cursor sessions". Kept as the 2026-10-02 decision record.
4. **Opt-in by default**: raw transcripts / global memory are privacy-sensitive → `RegisterSourceManual` excludes them from startup auto-import (`ImportAll` runs curated sources only). Global-memory sources attach to a config-named target project (`openclaw_project`/`hermes_project`); unset → skipped.
5. **Security**: OpenClaw/Hermes homes contain private keys / `.env` / vaults → importers are **hard-allowlisted to the one memory dir, non-recursive** (asserted by tests).

## Consequences

- Positive: multi-agent memory flows into one KB via a pattern every source reuses.
- Risk: each source is maintenance debt bound to an upstream format; mitigated by golden-file tests + lenient parse + verify-first gate.
