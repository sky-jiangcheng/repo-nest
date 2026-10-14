package runtime

import "testing"

// TestSourceTypeForBuiltinSources locks the derived display buckets for the
// eight built-in importers. The mapping mirrors each importer's artifact
// shape: curated agent-memory files are agent-import, session transcripts are
// user-conversation (see internal/importers/*/). This is the promise the
// settings page relies on for the per-source type chip.
func TestSourceTypeForBuiltinSources(t *testing.T) {
	cases := []struct {
		name string
		want SourceType
	}{
		{"claude", SourceTypeAgentImport},
		{"openclaw", SourceTypeAgentImport},
		{"hermes", SourceTypeAgentImport},
		{"codebuddy-memory", SourceTypeAgentImport},
		{"codex", SourceTypeConversation},
		{"opencode", SourceTypeConversation},
		{"cursor", SourceTypeConversation},
		{"codebuddy", SourceTypeConversation},
	}
	for _, tc := range cases {
		if got := sourceTypeFor(tc.name); got != tc.want {
			t.Errorf("sourceTypeFor(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestSourceTypeForFallback: a source outside the built-in set (future
// importer, test double, renamed source) must not produce an empty or
// misleading type — it falls back to Manual so the UI always has a label.
func TestSourceTypeForFallback(t *testing.T) {
	for _, name := range []string{"", "unknown", "my-plugin", "codebuddy-memory-old"} {
		if got := sourceTypeFor(name); got != SourceTypeManual {
			t.Errorf("sourceTypeFor(%q) = %q, want %q", name, got, SourceTypeManual)
		}
	}
}

// TestSourceStatusesCarryType: SourceStatuses must surface the derived type
// alongside the name, e.g. for the settings list.
func TestSourceStatusesCarryType(t *testing.T) {
	rt := New(setupDB(t))
	rt.RegisterSource("claude", staticImporter{source: "claude"})
	rt.RegisterSourceManual("codex", staticImporter{source: "codex"})
	rt.RegisterSourceManual("mystery", staticImporter{source: "mystery"})

	byName := map[string]SourceStatus{}
	for _, st := range rt.SourceStatuses() {
		byName[st.Name] = st
	}
	if got := byName["claude"].Type; got != SourceTypeAgentImport {
		t.Errorf("claude type = %q, want agent-import", got)
	}
	if got := byName["codex"].Type; got != SourceTypeConversation {
		t.Errorf("codex type = %q, want user-conversation", got)
	}
	if got := byName["mystery"].Type; got != SourceTypeManual {
		t.Errorf("mystery type = %q, want manual", got)
	}
}
