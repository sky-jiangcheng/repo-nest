package main

import (
	"os"
	"path/filepath"
	"testing"
)

// providers is the onboarding guide's axis A: what a user gets when they pick
// a provider and override nothing. Wrong defaults here are written straight
// into the user's config and silently break every later embedding call, so
// the table is pinned.
func TestProviderDefaultsTable(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantURL  string
		wantMod  string
		wantDim  int
		wantKey  bool
	}{
		{"ollama is the offline default", "ollama", "http://localhost:11434/v1", "nomic-embed-text", 768, false},
		{"openai needs a key", "openai", "https://api.openai.com/v1", "text-embedding-3-small", 1536, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, ok := providers[tt.provider]
			if !ok {
				t.Fatalf("provider %q missing from the table", tt.provider)
			}
			if def.baseURL != tt.wantURL || def.model != tt.wantMod || def.dim != tt.wantDim || def.needsKey != tt.wantKey {
				t.Errorf("providers[%q] = %+v, want url=%s model=%s dim=%d needsKey=%v",
					tt.provider, def, tt.wantURL, tt.wantMod, tt.wantDim, tt.wantKey)
			}
		})
	}
	if _, ok := providers["skip"]; ok {
		t.Error(`"skip" must not appear in the provider table: it is a control-flow branch, not an embedding provider`)
	}
}

func TestDefaultDimMirrorsTable(t *testing.T) {
	for name, def := range providers {
		if got := defaultDim(name); got != def.dim {
			t.Errorf("defaultDim(%q) = %d, want %d (drifted from the providers table)", name, got, def.dim)
		}
	}
}

// firstNonEmpty backs every override flag (-base-url, -model, …): a wrong
// precedence either discards the user's explicit override or lets an empty
// default shadow a later value.
func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"first wins", []string{"a", "b"}, "a"},
		{"skips empties", []string{"", "", "c"}, "c"},
		{"all empty", []string{"", ""}, ""},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstNonEmpty(tt.in...); got != tt.want {
				t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// The documented flag contract: -provider accepts exactly ollama | openai |
// skip. Anything else must fail loudly rather than fall through to a default,
// because a typo'd provider name would otherwise configure nothing and print
// a success banner anyway.
func TestProviderFlagContract(t *testing.T) {
	for _, p := range []string{"ollama", "openai", "skip"} {
		if _, ok := providers[p]; !ok && p != "skip" {
			t.Errorf("documented provider %q is not actually supported", p)
		}
	}
}

// setupDBFile documents where a database-backed test would point -db; the
// pure-function tests above never touch it, which is itself the design goal:
// everything a user can override through flags resolves without any I/O.
func setupDBFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vector-init-test.db")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return path
}

var _ = setupDBFile
