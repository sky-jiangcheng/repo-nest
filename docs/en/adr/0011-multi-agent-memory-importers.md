# ADR-0011: Multi-agent memory importers — reusable pipeline + per-source feasibility

- Status: Proposed (framework + five sources landed: claude AUTO; codex / opencode / openclaw / hermes (curated + sessions) as opt-in MANUAL. Cursor deferred after verification — see decision 3)
- Date: 2026-10-03
- Relates to: ADR-0002 (in-process plugin runtime), ADR-0007, ADR-0010, TODO M2. (Chinese original: `docs/adr/0011-*.md`.)

## Background

Repotory already has a clean pipeline: a `plugin.KnowledgeImporter` yields `[]plugin.ImportDoc` → registered via `Runtime.RegisterSource` → `upsertDoc` idempotently upserts notes keyed by (project, source, title). Originally only Claude was implemented. M2 extends it to other agents, whose on-disk formats are undocumented and drift.

## Decision

1. **Reuse the pipeline**, no new mechanism — each source is a Go `KnowledgeImporter`.
2. **Shared helpers extracted when the 2nd source landed** (`internal/importers/memsrc`: `MatchProject`, `ReadCapped`, `TargetProject`, `StripFrontmatter`, `ClipToBytes`).
3. **Per-source feasibility, verified against real files (never guessed):**
   - **codex** ✅ `~/.codex/sessions/**/rollout-*.jsonl` (streaming; `cwd`→project).
   - **opencode** ✅ `~/.local/share/opencode/storage/session/<hash>/ses_*.json` (already summarized: title/summary/directory).
   - **openclaw** ✅ global `~/.openclaw-autoclaw/workspace/*.md`.
   - **hermes** (Nous Research, separate product) ✅ curated `~/.hermes/memories/*.md` + sessions (since 2026-10-13, from `$HERMES_HOME/state.db`; see the implementation-correction note at the end of the Chinese ADR for the schema, verified against TencentDB-Agent-Memory's shipped asset importer).
   - **cursor** — verified → **deferred**: history is in an unofficial `state.vscdb` (bubble blobs + versioned headers + ProseMirror; no project path in-DB) → low-ROI/fragile.
4. **Opt-in by default**: raw transcripts / global memory are privacy-sensitive → `RegisterSourceManual` excludes them from startup auto-import (`ImportAll` runs curated sources only). Global-memory sources attach to a config-named target project (`openclaw_project`/`hermes_project`); unset → skipped.
5. **Security**: OpenClaw/Hermes homes contain private keys / `.env` / vaults → importers are **hard-allowlisted to the one memory dir, non-recursive** (asserted by tests).

## Consequences

- Positive: multi-agent memory flows into one KB via a pattern every source reuses.
- Risk: each source is maintenance debt bound to an upstream format; mitigated by golden-file tests + lenient parse + verify-first gate.
