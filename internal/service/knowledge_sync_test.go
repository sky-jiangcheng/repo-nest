package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"repotory/internal/core/plugin"
	pluginruntime "repotory/internal/core/plugin/runtime"
	"repotory/internal/db"
	"repotory/internal/importers/claude"
	"repotory/internal/stats"
)

// fakeSyncImporter is a controllable double for runKnowledgeSync: it records
// how often Import ran and reports a fixed Probe result.
type fakeSyncImporter struct {
	name     string
	detected bool
	err      error
	calls    int
}

func (f *fakeSyncImporter) Source() string { return f.name }
func (f *fakeSyncImporter) Import() ([]plugin.ImportDoc, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}
func (f *fakeSyncImporter) Probe() plugin.ProbeResult {
	return plugin.ProbeResult{Detected: f.detected, Detail: "fake for knowledge_sync tests"}
}

func TestSyncSourceEnabledDefaults(t *testing.T) {
	svc, _ := setupService(t)

	// The only source that defaults ON is the curated claude memory (it is
	// also the AUTO source of the startup pass); everything else starts manual.
	if !svc.syncSourceEnabled(claude.SourceName) {
		t.Errorf("%s should default to enabled", claude.SourceName)
	}
	for _, name := range []string{"codex", "opencode", "openclaw", "hermes", "cursor", "codebuddy", "codebuddy-memory"} {
		if svc.syncSourceEnabled(name) {
			t.Errorf("%s must default to disabled (transcript/agent-global sources are opt-in)", name)
		}
	}

	// Flipping the config row overrides the default in both directions.
	if err := svc.UpdateConfig("sync_source_"+claude.SourceName, "0"); err != nil {
		t.Fatalf("disable claude sync: %v", err)
	}
	if svc.syncSourceEnabled(claude.SourceName) {
		t.Error("claude sync should be off after UpdateConfig(\"0\")")
	}
	if err := svc.UpdateConfig("sync_source_codex", "1"); err != nil {
		t.Fatalf("enable codex sync: %v", err)
	}
	if !svc.syncSourceEnabled("codex") {
		t.Error("codex sync should be on after UpdateConfig(\"1\")")
	}
}

func TestKnowledgeSyncInterval(t *testing.T) {
	svc, _ := setupService(t)

	if got := svc.knowledgeSyncInterval(); got != 240*time.Minute {
		t.Errorf("default interval = %v, want 240m", got)
	}

	if err := svc.UpdateConfig("knowledge_sync_interval_minutes", "30"); err != nil {
		t.Fatalf("set interval: %v", err)
	}
	if got := svc.knowledgeSyncInterval(); got != 30*time.Minute {
		t.Errorf("interval = %v, want 30m", got)
	}

	// Invalid values fall back to the default rather than breaking the ticker.
	for _, bad := range []string{"abc", "0", "-5"} {
		if err := db.SetConfig(svc.db, "knowledge_sync_interval_minutes", bad); err != nil {
			t.Fatalf("SetConfig %q: %v", bad, err)
		}
		if got := svc.knowledgeSyncInterval(); got != 240*time.Minute {
			t.Errorf("interval with %q = %v, want default 240m", bad, got)
		}
	}
}

// TestSyncKeyRegistered ensures the settings page can actually flip the new
// keys: every sync key must pass the UpdateConfig allow-list and numeric gate.
func TestSyncKeyRegistered(t *testing.T) {
	svc, _ := setupService(t)
	for _, key := range []string{
		"knowledge_sync_interval_minutes",
		"sync_source_claude", "sync_source_codex", "sync_source_opencode",
		"sync_source_openclaw", "sync_source_hermes", "sync_source_cursor",
		"sync_source_codebuddy", "sync_source_codebuddy-memory",
	} {
		val := "1"
		if key == "knowledge_sync_interval_minutes" {
			val = "120"
		}
		if err := svc.UpdateConfig(key, val); err != nil {
			t.Errorf("UpdateConfig(%q) rejected: %v", key, err)
		}
	}
}

func TestKnowledgeSyncStatusInitiallyEmpty(t *testing.T) {
	svc, _ := setupService(t)
	st := svc.KnowledgeSyncStatus()
	if st.LastRun != "" || st.LastError != "" {
		t.Errorf("fresh status = %+v, want empty", st)
	}
}

func TestRunKnowledgeSync(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	rt := pluginruntime.New(database)
	enabled := &fakeSyncImporter{name: "fakesync-enabled", detected: true}
	undetected := &fakeSyncImporter{name: "fakesync-undetected", detected: false}
	broken := &fakeSyncImporter{name: "fakesync-broken", detected: true, err: errors.New("boom")}
	rt.RegisterSourceManual(enabled.name, enabled)
	rt.RegisterSourceManual(undetected.name, undetected)
	rt.RegisterSourceManual(broken.name, broken)

	svc := NewWithDeps(database, &fakeGit{ranges: map[string][]stats.DailyEntry{}}, rt, "me")
	// The fake names are not registered config keys (only the built-in eight
	// are), so write the rows directly — syncSourceEnabled reads the same path.
	for _, name := range []string{enabled.name, undetected.name, broken.name} {
		if err := db.SetConfig(database, "sync_source_"+name, "1"); err != nil {
			t.Fatalf("enable %s: %v", name, err)
		}
	}

	svc.runKnowledgeSync(context.Background())

	// Enabled+detected sources import; undetected ones skip (nothing to read);
	// an error in one source must not stop the others, and it lands in the
	// round's aggregate status.
	if enabled.calls != 1 || broken.calls != 1 {
		t.Errorf("enabled/broken imports = %d/%d, want 1/1", enabled.calls, broken.calls)
	}
	if undetected.calls != 0 {
		t.Errorf("undetected source imported %d times, want 0 (skip)", undetected.calls)
	}

	st := svc.KnowledgeSyncStatus()
	if st.LastRun == "" {
		t.Error("runKnowledgeSync did not record last_run")
	}
	if !strings.Contains(st.LastError, "boom") {
		t.Errorf("last_error = %q, want aggregate \"boom\"", st.LastError)
	}
}
