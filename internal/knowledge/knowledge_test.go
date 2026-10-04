package knowledge

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemp creates a temp directory with the named files (path may contain
// subdirectories) and their contents, then returns the root.
func writeTemp(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0640); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestExtractREADME(t *testing.T) {
	root := writeTemp(t, map[string]string{
		"README.md": "# Title\n\nSome docs\n",
		"main.go":   "package main\n",
	})
	got, err := ExtractREADME(root)
	if err != nil {
		t.Fatalf("ExtractREADME: %v", err)
	}
	if !strings.Contains(got, "# Title") {
		t.Errorf("README excerpt should contain the heading, got %q", got)
	}
}

func TestExtractREADMENoReadme(t *testing.T) {
	// No README present: empty excerpt, no error (documented behaviour).
	got, err := ExtractREADME(t.TempDir())
	if err != nil {
		t.Fatalf("ExtractREADME without README should not error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty excerpt, got %q", got)
	}
}

// Regression (same class as the Claude importer OOM fix): dependency
// manifests are read from repositories the user scans, so a pathological
// package.json (vendored, generated, accidental) must not be loaded whole.
// Deeper than the cap, a manifest is skipped; below it, deps still parse.
func TestParseDepsCappedRead(t *testing.T) {
	root := t.TempDir()

	// go.mod: deps within the first MB, then a huge trailing block that
	// pushes the file past the read cap. Line parsing tolerates truncation,
	// so the real deps must still be found.
	var goMod strings.Builder
	goMod.WriteString("module x\n\ngo 1.25\n\nrequire (\n\tgithub.com/spf13/cobra v1.9.1\n)\n")
	goMod.WriteString("// filler\n" + strings.Repeat("x", maxManifestReadBytes))
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod.String()), 0640); err != nil {
		t.Fatal(err)
	}
	deps, err := parseGoDeps(root)
	if err != nil {
		t.Fatalf("parseGoDeps: %v", err)
	}
	if len(deps) != 1 || deps[0].Name != "github.com/spf13/cobra" {
		t.Errorf("truncated go.mod should still yield its deps, got %+v", deps)
	}

	// package.json: valid JSON prefix followed by filler that pushes the
	// file past the cap. The truncated remainder is not parseable JSON, so
	// the manifest must be skipped (error) instead of read whole.
	pkg := `{"name":"x","dependencies":{"react":"^19.0.0"}}` + strings.Repeat("x", maxManifestReadBytes)
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(pkg), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := parseNpmDeps(root, "package.json"); err == nil {
		t.Error("oversized package.json must be skipped, not parsed")
	}
}

func TestDetectTechStack(t *testing.T) {
	root := writeTemp(t, map[string]string{
		"package.json":       `{"name":"x","dependencies":{"react":"^19.0.0"}}`,
		"go.mod":             "module x\n\ngo 1.25\n",
		"Dockerfile":         "FROM alpine\n",
		"not-a-manifest.txt": "ignore me",
	})
	techs, err := DetectTechStack(root)
	if err != nil {
		t.Fatalf("DetectTechStack: %v", err)
	}
	found := map[string]bool{}
	for _, tech := range techs {
		found[tech.Name] = true
	}
	for _, want := range []string{"JavaScript / TypeScript", "Go", "Docker"} {
		if !found[want] {
			t.Errorf("expected tech %q in detected stack, got %v", want, techs)
		}
	}
}

func TestDetectLanguages(t *testing.T) {
	root := writeTemp(t, map[string]string{
		"a.go": "package a\n",
		"b.go": "package b\n",
		"c.ts": "const x = 1\n",
	})
	langs, err := DetectLanguages(root)
	if err != nil {
		t.Fatalf("DetectLanguages: %v", err)
	}
	byLang := map[string]int{}
	for _, l := range langs {
		byLang[l.Language] = l.Count
	}
	if byLang["Go"] == 0 || byLang["TypeScript"] == 0 {
		t.Errorf("expected Go and TypeScript to be detected, got %v", byLang)
	}
	if byLang["Go"] != byLang["TypeScript"]*2 {
		t.Errorf("Go has twice the lines of TypeScript (2 vs 1 line files), got %v", byLang)
	}
}

func TestDetectDependenciesNpm(t *testing.T) {
	root := writeTemp(t, map[string]string{
		"package.json": `{
			"dependencies": {"react": "^19.0.0", "vite": "^8.0.0"},
			"devDependencies": {"typescript": "^7.0.0"}
		}`,
	})
	deps, err := DetectDependencies(root)
	if err != nil {
		t.Fatalf("DetectDependencies: %v", err)
	}
	names := map[string]string{}
	for _, d := range deps {
		names[d.Name] = d.Version
	}
	if names["react"] != "^19.0.0" {
		t.Errorf("expected react ^19.0.0, got %v", names)
	}
	if _, ok := names["typescript"]; !ok {
		t.Errorf("expected devDependencies to be included, got %v", names)
	}
}

func TestDetectDependenciesGo(t *testing.T) {
	root := writeTemp(t, map[string]string{
		"go.mod": "module reponest\n\ngo 1.25.5\n\nrequire (\n\tgithub.com/spf13/cobra v1.8.0\n)\n",
	})
	deps, err := DetectDependencies(root)
	if err != nil {
		t.Fatalf("DetectDependencies: %v", err)
	}
	if len(deps) == 0 {
		t.Fatal("expected at least one go dependency")
	}
	found := false
	for _, d := range deps {
		if d.Name == "github.com/spf13/cobra" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected cobra in deps, got %v", deps)
	}
}

func TestDetectDependenciesCargo(t *testing.T) {
	root := writeTemp(t, map[string]string{
		"Cargo.toml": "[dependencies]\nserde = \"1.0\"\n",
	})
	deps, err := DetectDependencies(root)
	if err != nil {
		t.Fatalf("DetectDependencies: %v", err)
	}
	if len(deps) != 1 || deps[0].Name != "serde" {
		t.Errorf("expected serde dep, got %v", deps)
	}
}

func TestDetectDependenciesNone(t *testing.T) {
	root := writeTemp(t, map[string]string{"main.go": "package main\n"})
	deps, err := DetectDependencies(root)
	if err != nil {
		t.Fatalf("DetectDependencies: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected no deps, got %v", deps)
	}
}

func TestDetectDependenciesGoBlock(t *testing.T) {
	// gofmt-standard block form must parse without panicking and find deps.
	root := writeTemp(t, map[string]string{
		"go.mod": `module reponest

go 1.25.5

require (
	github.com/spf13/cobra v1.8.0
	github.com/mark3labs/mcp-go v0.32.0
)
`,
	})
	deps, err := DetectDependencies(root)
	if err != nil {
		t.Fatalf("DetectDependencies: %v", err)
	}
	names := map[string]bool{}
	for _, d := range deps {
		names[d.Name] = true
	}
	if !names["github.com/spf13/cobra"] || !names["github.com/mark3labs/mcp-go"] {
		t.Errorf("block-form requires should be parsed, got %v", deps)
	}
}

func TestDetectDependenciesGoRequireSingleAndBlock(t *testing.T) {
	root := writeTemp(t, map[string]string{
		"go.mod": `module x

go 1.25

require github.com/single/one v1.0.0

require (
	github.com/block/two v2.0.0
)
`,
	})
	deps, err := DetectDependencies(root)
	if err != nil {
		t.Fatalf("DetectDependencies: %v", err)
	}
	if len(deps) != 2 {
		t.Errorf("expected 2 deps (single + block), got %v", deps)
	}
}

// initGitRepo creates a real git repository with one commit per (author,
// message) pair, so git-backed detectors run against a genuine history.
func initGitRepo(t *testing.T, commits map[string]string) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=tester", "GIT_AUTHOR_EMAIL=tester@example.com",
			"GIT_COMMITTER_NAME=tester", "GIT_COMMITTER_EMAIL=tester@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.name", "tester")
	run("config", "user.email", "tester@example.com")
	for msg, author := range commits {
		if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte(msg), 0640); err != nil {
			t.Fatal(err)
		}
		run("add", ".")
		// Per-commit author via env: map iteration order is random, so the
		// counts must be asserted through the parser, not map order.
		cmd := exec.Command("git", "commit", "-q", "-m", msg)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME="+author, "GIT_AUTHOR_EMAIL="+author+"@example.com",
			"GIT_COMMITTER_NAME="+author, "GIT_COMMITTER_EMAIL="+author+"@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v: %s", err, out)
		}
	}
	return root
}

// Regression: shortlog without a rev argument reads its log from stdin, which
// is /dev/null for a nil exec.Cmd — the output was always empty and the
// contributor list was silently empty forever. `-n<limit>` also meant
// rev-list --max-count (scan only N commits), not "top N authors".
func TestDetectContributorsReturnsAuthors(t *testing.T) {
	root := initGitRepo(t, map[string]string{
		"first":  "alice",
		"second": "alice",
		"third":  "bob",
	})

	got, err := DetectContributors(root, 5)
	if err != nil {
		t.Fatalf("DetectContributors: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 contributors, got %+v", got)
	}
	if got[0].Author != "alice" || got[0].Count != 2 {
		t.Errorf("top contributor should be alice (2 commits), got %+v", got[0])
	}
	if got[1].Author != "bob" || got[1].Count != 1 {
		t.Errorf("second contributor should be bob (1 commit), got %+v", got[1])
	}

	// limit truncates after counting, not before.
	if got, err := DetectContributors(root, 1); err != nil || len(got) != 1 {
		t.Errorf("limit=1 should truncate to the top author, got %+v err=%v", got, err)
	}
}
