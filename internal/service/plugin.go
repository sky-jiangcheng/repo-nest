package service

import (
	"encoding/json"
	"fmt"

	pluginruntime "repo-nest/internal/core/plugin/runtime"
	"repo-nest/internal/importers/claude"
	"repo-nest/internal/importers/codex"
	"repo-nest/internal/importers/cursor"
	"repo-nest/internal/importers/hermes"
	"repo-nest/internal/importers/openclaw"
	"repo-nest/internal/importers/opencode"
	"repo-nest/internal/platform"
)

// pluginsDir resolves the plugin directory; a thin seam over platform so the
// service package stays testable without touching the real config dir in unit
// tests that do not call Startup.
func pluginsDir() string { return platform.GetPluginsDir() }

// jsonUnmarshal decodes data into v, returning whether it succeeded. Cached
// repo_meta payloads may be legacy or empty; failures are non-fatal.
func jsonUnmarshal(data string, v any) bool {
	return json.Unmarshal([]byte(data), v) == nil
}

// GetPluginStatuses returns the load status of every plugin directory.
func (s *Service) GetPluginStatuses() []pluginruntime.PluginStatus {
	if s.rt == nil {
		return []pluginruntime.PluginStatus{}
	}
	return s.rt.PluginStatuses()
}

// GetKnowledgeSources returns registered knowledge importers with their state.
func (s *Service) GetKnowledgeSources() []pluginruntime.SourceStatus {
	if s.rt == nil {
		return []pluginruntime.SourceStatus{}
	}
	return s.rt.SourceStatuses()
}

// emitImportEvent forwards an import result both to the plugin event bus and
// to the registered UI handler (which forwards it to the frontend).
func (s *Service) emitImportEvent(name string, run pluginruntime.ImportRun, err error) {
	if s.rt != nil {
		s.rt.Emit("import.completed", map[string]any{
			"source": name, "created": run.Created, "updated": run.Updated, "skipped": run.Skipped,
		})
	}
	if s.onImportEvent != nil {
		payload := ImportEventPayload{"source": name, "created": run.Created, "updated": run.Updated, "skipped": run.Skipped}
		if err != nil {
			payload["error"] = err.Error()
		}
		s.onImportEvent(payload)
	}
}

// TriggerKnowledgeImport runs the knowledge source registered under name and
// returns the import statistics.
func (s *Service) TriggerKnowledgeImport(name string) (pluginruntime.ImportRun, error) {
	if s.rt == nil {
		return pluginruntime.ImportRun{}, fmt.Errorf("plugin runtime not initialized")
	}
	run, err := s.rt.TriggerImport(name)
	s.emitImportEvent(name, run, err)
	return run, err
}

// TriggerAllKnowledgeImports runs every registered knowledge source and emits
// an import.completed event for each, so the UI can surface per-source results.
func (s *Service) TriggerAllKnowledgeImports() []pluginruntime.SourceRun {
	if s.rt == nil {
		return []pluginruntime.SourceRun{}
	}
	results := s.rt.ImportAll()
	for _, r := range results {
		var err error
		if r.Err != "" {
			err = fmt.Errorf("%s", r.Err)
		}
		s.emitImportEvent(r.Name, r.Run, err)
	}
	return results
}

// ReloadPlugins rescans the plugins directory and reloads every plugin. The
// built-in importers are re-registered afterwards because Runtime.Load resets
// the source map.
func (s *Service) ReloadPlugins() []pluginruntime.PluginStatus {
	if s.rt == nil {
		return []pluginruntime.PluginStatus{}
	}
	s.rt.Load(pluginsDir())
	s.registerBuiltinImporters()
	return s.rt.PluginStatuses()
}

// registerBuiltinImporters registers the built-in knowledge importers with the
// plugin runtime. Sources are registered with different auto-import policies:
//   - claude:   AUTO — it reads curated ~/.claude/.../memory/*.md the user keeps.
//   - codex:    MANUAL — raw ~/.codex session transcripts, excluded from the
//     startup auto-import; only runs when triggered explicitly by name.
//   - opencode: MANUAL — ~/.local/share/opencode session summaries, same gate.
//   - openclaw: MANUAL — agent-GLOBAL ~/.openclaw-autoclaw/workspace/*.md,
//     allowlisted to that dir (parent holds keys/vault); attaches to the
//     `openclaw_project` config-named project (unset -> skipped).
//   - hermes:   MANUAL — agent-GLOBAL ~/.hermes/memories/*.md (Nous Research,
//     a product separate from OpenClaw), allowlisted to memories/; `hermes_project`.
//   - cursor:   MANUAL — Cursor session transcripts from its globalStorage SQLite
//     state DB, read-only. BEST EFFORT by design (undocumented, versioned format):
//     an unrecognized layout fails the source loudly instead of importing zero
//     notes quietly. Sessions are attributed through workspaceStorage folder
//     lookup, so an unmatched workspace skips rather than guessing.
//
// Manual sources still appear in GetKnowledgeSources, so each stays one-click
// triggerable. ADR-0011 决策 4.
func (s *Service) registerBuiltinImporters() {
	s.rt.RegisterSource(claude.SourceName, claude.New(s.db))
	s.rt.RegisterSourceManual(codex.SourceName, codex.New(s.db))
	s.rt.RegisterSourceManual(opencode.SourceName, opencode.New(s.db))
	s.rt.RegisterSourceManual(openclaw.SourceName, openclaw.New(s.db))
	s.rt.RegisterSourceManual(hermes.SourceName, hermes.New(s.db))
	s.rt.RegisterSourceManual(cursor.SourceName, cursor.New(s.db))
}

// ImportResult summarizes a Claude memory import run.
type ImportResult struct {
	Synced  int `json:"synced"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

// ImportClaudeMemory imports notes from Claude's per-project memory directory
// (~/.claude/projects/*/memory/*.md) into RepoNest, matching each to a project
// by name or repository path. The import is delegated to the built-in Claude
// KnowledgeImporter through the plugin runtime, so it is idempotent and shares
// the same upsert and statistics path as script plugins (issue #35).
func (s *Service) ImportClaudeMemory() (*ImportResult, error) {
	if s.rt == nil {
		return &ImportResult{}, nil
	}
	run, err := s.rt.TriggerImport(claude.SourceName)
	if err != nil {
		return &ImportResult{}, err
	}
	return &ImportResult{
		Synced:  run.Created,
		Updated: run.Updated,
		Skipped: run.Skipped,
	}, nil
}
