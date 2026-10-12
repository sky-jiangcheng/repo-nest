# ADR-0010: Session auto-capture — zero-touch Claude Code session handoff (M1)

- Status: Accepted-in-principle (parser + on-demand capture — service/binding/frontend — and the B-end `repotory-capture` hook CLI are all landed; `claude_session_capture` default-off)
- Date: 2026-10-03
- Relates to: ADR-0007 (context/handoff protocol), ADR-0009 (SessionEnd hook), TODO M1. (Chinese original: `docs/adr/0010-*.md`.)

## Background

Repotory handoffs today are **agent-pushed**: the agent calls `repotory_handoff` and `service.CreateHandoffNote` stores a handoff-tagged note. If the agent forgets, the session is lost. M1 closes that by having Repotory read Claude Code's own transcript at `~/.claude/projects/<slug>/<id>.jsonl` and produce a handoff itself — distinct from the Claude **memory** importer (which reads curated `memory/*.md`).

## Decision

1. **Segmented by user:** C-end (personal) — **default OFF**, on-demand only (a Settings toggle + "capture by project ID"); B-end (enterprise, own risk controls) — may enable the `repotory-capture` SessionEnd-hook automation, but the privacy risk of reading transcripts is documented and owned by them.
2. **Default off + explicit enable.** Reading raw conversation transcripts is a privacy-sensitive capability; never implicitly on.
3. **Local read-only + path allowlist**: only the project's `<slug>/*.jsonl`; never uploaded, never in telemetry.
4. **Bounded extraction**: last assistant text + tool-call summary + cwd/gitBranch; never archive the full transcript.
5. **Format-drift defence**: lenient parse (skip unknown/oversized lines), golden-file tests.

## Landed (2026-10-03)

`internal/importers/claude/session.go` (`ParseSession`/`LatestSessionForRootPath`, live-verified against the real v2.1.278 format); `service.CaptureClaudeHandoff` / `CaptureClaudeSessionByCwd` (config-gated, `Session→HandoffInput` → shared `CreateHandoffNote`, tagged `auto-captured`); desktop bindings + `Settings→Plugins` toggle/capture UI + `endpoints.captureClaudeHandoff`; `cmd/repotory-capture` (SessionEnd hook target, exit 0 when disabled so it never breaks session teardown). Remaining: none for the on-demand path; the B-end hook is wired via `repotory-init`.

## Consequences

- Positive: the memory loop closes even when the agent forgets to call the tool.
- Risk: bound to a non-public transcript format; the mechanically-extracted handoff is a lower-signal safety net than an agent-authored one (positioned as complement, not replacement).
