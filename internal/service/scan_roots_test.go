package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNormalizeScanRoots covers the four defects found in the settings-page
// round of UI testing:
//
//	P1  duplicate entries reached the DB and collided as React list keys
//	P2  a trailing slash made the same directory two distinct roots
//	P3  "", "   ", "./relative" and "~" paths were stored without validation
//
// Cases run through a real temp directory so the filesystem checks are
// exercised rather than stubbed.
func TestNormalizeScanRoots(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "repo")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(base, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(base, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		input     []string
		wantRoots []string
		wantRej   map[string]string // path -> reason
	}{
		{
			name:      "plain absolute path is kept as-is",
			input:     []string{real},
			wantRoots: []string{real},
		},
		{
			name:      "trailing slash collapses to one root (P2)",
			input:     []string{real + "/", real},
			wantRoots: []string{real},
			wantRej:   map[string]string{real: "duplicate"},
		},
		{
			name:      "dot and dotdot segments are cleaned",
			input:     []string{filepath.Join(base, "other", "..", "repo")},
			wantRoots: []string{real},
		},
		{
			name:    "empty and whitespace-only are refused (P3)",
			input:   []string{"", "   ", "\t\n"},
			wantRej: map[string]string{"": "empty", "   ": "empty"},
		},
		{
			name:    "relative paths are refused (P3)",
			input:   []string{"./relative", "../up", filepath.Join(base, "..", "relative-x")},
			wantRej: map[string]string{"./relative": "relative", "../up": "relative"},
		},
		{
			name:    "nonexistent path is refused (P3)",
			input:   []string{filepath.Join(base, "nope")},
			wantRej: map[string]string{filepath.Join(base, "nope"): "not_found"},
		},
		{
			name:    "a file is not a directory",
			input:   []string{file},
			wantRej: map[string]string{file: "not_a_directory"},
		},
		{
			name:      "tilde expands to home",
			input:     []string{"~"},
			wantRoots: []string{strings.TrimSuffix(homeOrSkip(t), string(os.PathSeparator))},
		},
		{
			name:      "duplicate submitted twice (P1)",
			input:     []string{real, real},
			wantRoots: []string{real},
			wantRej:   map[string]string{real: "duplicate"},
		},
		{
			name:      "good roots survive alongside bad ones",
			input:     []string{real, "", "nope-relative", other},
			wantRoots: []string{real, other},
			wantRej:   map[string]string{"": "empty", "nope-relative": "relative"},
		},
		{
			name:      "duplicate detection folds case on case-insensitive platforms",
			input:     []string{real, strings.ToUpper(real)},
			wantRoots: []string{real},
			wantRej:   map[string]string{strings.ToUpper(real): "duplicate"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			roots, rejected := normalizeScanRoots(tc.input)

			if len(roots) != len(tc.wantRoots) {
				t.Fatalf("roots = %q, want %q (rejected %+v)", roots, tc.wantRoots, rejected)
			}
			for i, r := range roots {
				if r != tc.wantRoots[i] {
					t.Errorf("roots[%d] = %q, want %q", i, r, tc.wantRoots[i])
				}
			}

			// Rejection paths are Clean-ed for display, so match on the reason
			// attached to each path rather than the raw submission.
			for path, reason := range tc.wantRej {
				found := false
				for _, rj := range rejected {
					if rj.Reason != reason {
						continue
					}
					if path == "" || rj.Path == filepath.Clean(path) || rj.Path == path {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no rejection with reason %q for %q; got %+v", reason, path, rejected)
				}
			}
		})
	}
}

func homeOrSkip(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory in this environment")
	}
	return home
}

// TestNormalizeScanRoots_EmptyInput pins the JSON contract: ScanRoots must be an
// empty array, never null, or the settings page throws while spreading it.
func TestNormalizeScanRoots_EmptyInput(t *testing.T) {
	roots, rejected := normalizeScanRoots(nil)
	if roots != nil {
		t.Errorf("roots = %v, want nil (caller substitutes an empty slice)", roots)
	}
	if len(rejected) != 0 {
		t.Errorf("rejected = %+v, want empty", rejected)
	}
}

// TestNormalizeScanRoots_CaseFoldingPlatform documents the one place where
// case-insensitive dedup is a deliberate exception rather than a bug.
func TestNormalizeScanRoots_CaseFoldingPlatform(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("case-folding only applies to Windows and macOS")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "Repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	roots, rejected := normalizeScanRoots([]string{dir, strings.ToUpper(dir)})
	if len(roots) != 1 {
		t.Fatalf("roots = %q, want exactly 1 entry", roots)
	}
	if len(rejected) != 1 || rejected[0].Reason != "duplicate" {
		t.Errorf("rejected = %+v, want one duplicate", rejected)
	}
}

// TestUpdateScanRoots_PersistsNormalizedList is the service-level regression:
// what the backend stores must equal what it reports back, and refused entries
// must not reach the database.
func TestUpdateScanRoots_PersistsNormalizedList(t *testing.T) {
	svc, _ := setupService(t)
	base := t.TempDir()
	real := filepath.Join(base, "repo")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := svc.UpdateScanRoots([]string{real + "/", "", filepath.Join(base, "missing")})
	if err != nil {
		t.Fatalf("UpdateScanRoots: %v", err)
	}
	if len(res.ScanRoots) != 1 || res.ScanRoots[0] != real {
		t.Errorf("ScanRoots = %q, want [%q] (trailing slash cleaned)", res.ScanRoots, real)
	}
	if len(res.Rejected) != 2 {
		t.Errorf("Rejected = %+v, want 2 entries", res.Rejected)
	}

	// The reported list and the persisted list must agree.
	cfg, err := svc.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if len(cfg.ScanRoots) != 1 || cfg.ScanRoots[0] != real {
		t.Errorf("persisted roots = %q, want [%q]", cfg.ScanRoots, real)
	}
}

// TestUpdateScanRoots_EmptyListClears verifies that clearing every root still
// succeeds and persists nothing, rather than being refused by the new
// validation (an empty list must be a legitimate "user cleared everything").
func TestUpdateScanRoots_EmptyListClears(t *testing.T) {
	svc, _ := setupService(t)
	base := t.TempDir()
	real := filepath.Join(base, "repo")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateScanRoots([]string{real}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := svc.UpdateScanRoots([]string{})
	if err != nil {
		t.Fatalf("clearing roots must succeed, got: %v", err)
	}
	if len(res.ScanRoots) != 0 {
		t.Errorf("ScanRoots = %q, want empty", res.ScanRoots)
	}
	cfg, err := svc.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if len(cfg.ScanRoots) != 0 {
		t.Errorf("persisted roots = %q, want empty", cfg.ScanRoots)
	}
}

// TestUpdateScanRoots_NilNeverSerialisesNull guards the payload shape the
// frontend depends on: scan_roots and rejected must both be present as arrays.
func TestUpdateScanRoots_NilNeverSerialisesNull(t *testing.T) {
	svc, _ := setupService(t)
	res, err := svc.UpdateScanRoots([]string{})
	if err != nil {
		t.Fatalf("UpdateScanRoots: %v", err)
	}
	if res.ScanRoots == nil {
		t.Error("ScanRoots is nil -> serialises to null")
	}
	if res.Rejected == nil {
		t.Error("Rejected is nil -> serialises to null")
	}
}
