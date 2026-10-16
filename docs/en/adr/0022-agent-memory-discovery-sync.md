# ADR-0022: Auto-discovery and scheduled sync of agent memory — source type visible to the user

- Status: Proposed (design pending review; the CodeBuddy wiring is an independent first-step item that can land without waiting for full approval)
- Date: 2026-10-14
- Related: [ADR-0011](0011-multi-agent-memory-importers.md) (multi-agent memory importers), [ADR-0014](0014-llm-wiki-knowledge-compiler.md) (LLM Wiki knowledge compiler), [ADR-0015](0015-w3-minimal-compile-loop.md) (W3 minimal compile loop), [ADR-0007](0007-session-memory-protocol.md) (session memory protocol), [ADR-0009](0009-ide-presence.md) (IDE presence), TODO session-memory roadmap M2/M8

## Background

User request (target environment is a **macOS host machine**, the host application is a Wails desktop app; agent memory directories differ by OS, discovery paths are OS-specific, this ADR uses macOS as the reference): the local machine has multiple agent tools with built-in conversational memory — CodeBuddy, an **IDEA/VS Code/JetBrains plugin** (session transcripts land in `~/Library/Application Support/CodeBuddyExtension/Data/...` and curated memory in `~/.codebuddy/memery/*.md`), plus **command-line agents** such as Claude Code / Codex / OpenCode / OpenClaw / Hermes / Cursor. The user wants the application to do four things:

1. **Automatically discover** conversational memory from these agent tools on the local machine;
2. **Periodically sync** that memory;
3. **Organize** the synced content into the application's own knowledge base;
4. Make the **source type of each knowledge item visible to the user** (e.g. external import / project extraction / user conversation).

The 2026-10-14 inventory shows that points 3 and 4 already have groundwork; points 1 and 2 are pure gaps:

- **Import pipeline is ready (ADR-0011)**: `plugin.KnowledgeImporter` produces `[]plugin.ImportDoc{ProjectID,Title,Content,Tags,Kind,Source}`, registered via `Runtime.RegisterSource(/Manual)` (`internal/core/plugin/runtime/runtime.go`); `TriggerImport → upsertDoc` idempotently upserts into `project_notes` keyed on `(project_id, source, title)` (`source TEXT DEFAULT 'manual'`). Six sources are registered: `claude` (AUTO, the only auto-import on startup) and `codex/opencode/openclaw/hermes/cursor` (MANUAL, default off, explicit trigger only). `internal/service/plugin.go:89 registerBuiltinImporters` is the single registration point.
- **CodeBuddy importer is written but not wired in, not committed**: `internal/importers/codebuddy/codebuddy.go` (621 lines, untracked by git) implements two sources — `codebuddy` (session transcripts) + `codebuddy-memory` (curated memory, `codebuddy_project` target-project config) — but `plugin.go` has no import or registration for it; the comment still says "six builtin sources." This is the only half-finished operation in the current repository.
- **No auto-discovery**: source availability depends on the user knowing the paths; `SourceStatus.Enabled` is currently just a derived value of `lastErr == nil`, not a detection of "is this agent installed locally and is the memory directory readable." After installing CodeBuddy, the settings page will not spontaneously surface the source.
- **No scheduled sync**: only the startup auto-import (claude only) and manual button presses in settings; the service layer already has a `time.NewTicker` convention (`startEmbedDrainer` 5 s, `startWikiLintTicker` 6 h, page drain, compile poll — all in `internal/service/`), but knowledge import has no periodic channel.
- **Source annotation is just a raw string, no classification, nearly invisible**: `project_notes.source` stores the importer name (claude/codex/…/manual); the front-end only draws a badge for `source==='claude'` at `NoteSection.tsx:274` — all other sources, `manual`, and `handoff` (session handoff) are not shown; there is no "external import / project extraction / user conversation" classification concept and no source-filter entry point. The scan pipeline only produces `repo_meta` (README excerpt + stats, fed into LLM context), not knowledge notes, so "project extraction" is currently an empty category that needs to be reserved.

## Decisions

1. **Discovery = each source implements `Probe()`, register-and-probe**. Add `Probe() ProbeResult` to `plugin.KnowledgeImporter` (`Detected bool` + `Detail`: root path, matched file count, error summary); `Runtime` runs a read-only probe on every source at `RegisterSource` time; `SourceStatus` is extended to `{name, plugin, detected, enabled, detail}` (`enabled` keeps its original meaning: whether the last import succeeded). Probing follows the **hard-scoping discipline** from openclaw/hermes: check only whether at least one memory file exists inside the source's declared root directory (e.g. `~/.codebuddy/...`, `~/.claude/...`); never read outside the parent/sibling/subdirectory. The settings-page source list changes from "list the registry" to "what was actually detected on this machine."

2. **CodeBuddy wiring is the first new source (first-step item)**. Commit `codebuddy.go` into the repo; add `RegisterSourceManual(codebuddy.SourceName, codebuddy.New(s.db))` and `RegisterSourceManual(codebuddy.MemorySourceName, codebuddy.NewMemory(s.db))` to `registerBuiltinImporters` (keep MANUAL, session transcripts are privacy-sensitive, ADR-0011 decision 4 defaults off); add `codebuddy_project` to the "global memory source target project" input group alongside `openclaw_project`/`hermes_project` on the settings page; update the "six" comments in `plugin.go`/`runtime.go` to "eight." Cost: about one commit + ten lines of registration; fastest way to clear the "unfinished operation."

3. **Scheduled sync = service-layer sync ticker + per-source persistent switch, default off**. Add `internal/service/knowledge_sync.go`: `startKnowledgeSyncTicker` (follows the `time.NewTicker` convention, **first tick deferred by one period** to avoid full import on startup, same as `startWikiLintTicker`). Parameters:
   - Period config `knowledge_sync_interval_minutes` (default `240`, i.e. 4 hours);
   - Per-source switch `sync_source_<name>` (`'1'`/`'0'`, default `'0'` off; `claude` keeps AUTO semantics with default `'1'`) — **privacy default-off** extends ADR-0011 decision 4 ("every source defaults off, explicitly turned on") to the periodic channel;
   - Each sync round = `TriggerImport` sequentially for all `sync_source_*='1'` sources (`importMu` already serializes; mutually exclusive with the settings-page manual import, no duplication);
   - Results go through the existing `emitImportEvent → import.completed` toast, and `SourceStatus` carries `lastRun`/`lastError` for the settings page to display the last sync time and error.

4. **Source type = server-side derived classification**. Add an enum (no new DB column; `project_notes.source` already exists; classification is display/filtering semantics derived from the stored value):

   | Source type | Meaning (user-visible label) | Mapped source values |
   |---|---|---|
   | `agent-import` | External import (agent memory) | `claude, codex, opencode, openclaw, hermes, cursor, codebuddy, codebuddy-memory` |
   | `user-conversation` | User conversation (session handoff) | `handoff` (existing handoff-tagged notes), future structured conversation imports |
   | `project-extract` | Project extraction (from project/repo) | Reserved empty category; current scan produces `repo_meta` only, not notes; future README/code summaries compiled into notes fall here |
   | `manual` | Manual (user-written/edited) | `manual` and user-created notes |

   Implemented as a single `SourceType(source string) (type, label)` function in `internal/service` (or `internal/core`); unknown sources fall back to displaying the raw string with type `manual`; the front-end receives `{source, source_type, label}` through existing bindings; i18n labels go through react-i18next.

5. **Source type visible to user = three-layer UI**:
   - **Note card**: generalize the existing claude-only badge in `NoteSection.tsx` (`badgeSource`) into a "source badge" — every source displays its type label (e.g. "CodeBuddy import", "Session handoff", "Manual") + type color, alongside the existing kind badge;
   - **Settings page source list**: enhance each row in `PluginsTab.tsx` to: detection status (detected/undetected + root path) → source type → scheduled sync switch → import now → last sync time; the global memory source target-project input gains `codebuddy_project` per decision 2;
   - **Knowledge browse/search**: the note list and search results reuse the same source-badge component; the user can answer "where does this knowledge come from" on any screen that shows knowledge.

6. **Organizing into the own knowledge base = keep the two-layer pipeline, no new storage**. Import → `project_notes` (existing upsertDoc) → M6 wiki compilation (ADR-0014/0015) produces wiki pages (`page.source` inherits the note's source); source type is visible at both the notes and pages layers. This ADR adds no new tables, does not rebuild indexes, and does not touch the `embed-drainer`/vector search (new notes from sync naturally enter search via the `note_embed_dirty` queue).

## Rationale

- **Reuse over new-build**: discovery (Probe) = one read-only function per source; sync = one ticker + a set of config keys; source type = one derived function + badge generalization. All falls within the ADR-0011 pipeline and the service-layer ticker convention; no schema changes, no layering freeze violation (ADR-0006).
- **Privacy default-off continues the existing discipline**: scheduled sync is a high-sensitivity capability ("periodically reading agent private memory directories"); per-source explicit enablement is consistent with ADR-0011 decision 4, avoiding "open the app and all my agent conversations get imported."
- **Source classification via derived mapping rather than schema migration**: the `source` string is already a versioned fact (v23 and earlier); classification is display semantics; a single derived function keeps server and front-end consistent; when filtering/statistics by type becomes a frequent path, it can be materialized as a column (see open questions).
- **CodeBuddy first-step is natural**: the code is written, only wiring is missing; its dual-source shape (session transcripts + curated memory) pilots both "discovery" and "scheduled sync" forms, and directly clears the repository's only unfinished operation.

## Consequences

- **Positive**: the settings page goes from "I know which sources exist" to "the app tells me what's installed on this machine"; no hand-typed paths needed after installing a new agent; scheduled sync keeps agent memory flowing into the knowledge base incrementally and into semantic search via the existing embed-drainer; source type is visible in three places (notes, pages, settings), satisfying the first-class question "where did this knowledge come from."
- **Negative**: probing at registration adds one read-only I/O pass (one stat + a few file listings per source, acceptable); scheduled sync consumes periodic I/O/CPU proportional to the number of enabled sources, controlled by the interval config and per-source switches; source classification is a manual enum — new sources must register their mapping or they fall into `manual`/raw-string fallback.
- **Open questions**: ① Provide a "global one-click enable all source syncs" — leaning no, per-source switches better fit the privacy default-off; ② Add source-type filter and statistics to search/lists — wait for real demand signal; ③ Materialize `source_type` as a column (schema v24) — revisit when type-based filtering becomes a high-frequency path; ④ Backoff retry on sync failure — keep `TriggerImport` single-shot semantics + `lastErr` display for now, observe on real hardware.

## Phased delivery plan (for review)

- **P1 Wiring (first-step)**: `git add internal/importers/codebuddy/codebuddy.go`; register both sources in `plugin.go` + update comments; update `runtime.go` comments; add `codebuddy_project` to the `PluginsTab` target-project group. Verify: `go build ./...`, `go test ./internal/importers/codebuddy/ ./internal/core/plugin/runtime/`; run one CodeBuddy dual-source import on real hardware (requires CodeBuddy data present locally).
- **P2 Discovery (Probe)**: add the `Probe` contract to runtime and `SourceStatus.detected/detail`; implement read-only probes for seven sources (claude/codex/opencode/openclaw/hermes/cursor/codebuddy×2) — most check the root directory + at least one sample file, reusing each source's existing golden-file samples; display detection status on the settings page.
- **P3 Scheduled sync**: `knowledge_sync.go` ticker + `knowledge_sync_interval_minutes` + per-source `sync_source_<name>` + settings-page switch and lastRun display.
- **P4 Source type and UI**: `SourceType` derived function + tests (pin the mapping table above + unknown fallback); generalize the NoteSection source badge; enhance settings-page per-source type/switch info; i18n labels.
- **P5 Verification and wrap-up**: golden-file regression maintained (ADR-0011 decision 5); `go test ./internal/...`; GUI smoke test (source list discovery → enable scheduled sync for a source → note badge → settings lastRun); this ADR's promotion and implementation-correction notes are back-filled per project convention after delivery.
