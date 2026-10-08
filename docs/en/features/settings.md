---
title: Settings
order: 5
---

# Settings

The Settings page has six tabs: Scan Directories, Code Target, Author, Appearance, Plugins, and Actions.

## Scan Directories

- Add / remove scan roots (a rescan is required after saving for changes to take effect)
- For the default roots seeded automatically on first launch, see [Getting Started](../getting-started.md)

Only directories added here get scanned — which is exactly what step "① Configure scan directories" in [Getting Started](../getting-started.md#2-run-a-scan) is for.

## Code Target

| Setting | Description | Default |
|---------|-------------|---------|
| Daily goal (lines) | The workday check threshold; drives the progress ring and card alerts | 500 (range 100-10000) |
| Maximum scan depth | Directory levels to descend below each scan root | 2 (range 1-2) |

## Author

**Git author name**: the author filter for personal statistics (same semantics as `git log --author`). When unset, it is read automatically from `git config user.name`.

## Appearance

Light / Dark / Follow system — pick one; changes apply immediately and are remembered.

## Plugins

- **Auto-import toggle**: whether to run **auto-eligible** knowledge sources at startup — only "curated" sources (Claude memory) run in that pass; the other four stay manual. **Off by default**, and it only runs when explicitly enabled (turning it off does not stop you from importing any single source by hand on this page). Note this is a **behavior change**: upgrading normalizes an existing "on" to "off", because the old version could not distinguish "the user enabled it" from "the program seeded the default" — we would rather everyone click once again than have the app make that privacy decision on their behalf.
- **Claude session auto-capture (M1, off by default)**: a `claude_session_capture` toggle plus a "capture latest session by project ID" action — on-demand capture of a project's newest Claude Code session transcript into a handoff note. Nothing is read until you enable it; B-end users get a `reponest-capture` SessionEnd hook for automatic capture (see [ADR-0010](../adr/0010-session-auto-capture.md)).
- **Global-memory source target project**: OpenClaw / Hermes memory is **cross-project global**, so you pick which RepoNest project to attach it to (`openclaw_project` / `hermes_project` — name or ID); unset → that source skips, never mis-attached.
- **Knowledge import sources**: built-in + plugin-registered importers, each with an **Import Now** action. Six built-ins now — **claude** (eligible for startup auto-import, which only runs when the switch above is explicitly on) and **codex / opencode / openclaw / hermes / cursor** as **opt-in manual** sources (transcripts / global memory are sensitive → excluded from startup auto-import, triggered here or via config). `cursor` is best effort: an undocumented format, first prompt + last reply only, and it fails the whole source loudly rather than silently importing nothing when the layout is unrecognized. Remote vector stores (Qdrant / Weaviate) and the embedding provider are advanced config; see [ADR-0012](../adr/0012-semantic-search.md) / [ADR-0013](../adr/0013-vector-database-selection.md).
- **Loaded plugins**: load status and error messages per plugin directory; **Reload** hot-reloads.

> Privacy posture: transcript / global-memory sources are **off by default, explicit opt-in, directory-allowlisted** (never traversing parent dirs that hold keys/private material); semantic search is off by default and gated behind an A/B eval.

For plugin development, see the [Plugin Handbook](../plugins/overview.md).

## Actions

- **Rescan all projects now**: triggers a full scan (equivalent to the dashboard button)
- **Import Claude memory**: triggers one import manually; once it finishes, jump to the knowledge base to review
