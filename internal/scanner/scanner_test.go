package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// makeRepo creates a directory containing a .git subdirectory.
func makeRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0750); err != nil {
		t.Fatal(err)
	}
}

func TestScanRepositoriesFindsNestedRepos(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, filepath.Join(root, "alpha"))
	makeRepo(t, filepath.Join(root, "org", "beta"))
	makeRepo(t, filepath.Join(root, "org", "team", "gamma"))

	repos, err := ScanRepositories(context.Background(), []string{root}, 3)
	if err != nil {
		t.Fatalf("ScanRepositories: %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d: %+v", len(repos), repos)
	}
}

func TestScanRepositoriesDepthLimit(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, filepath.Join(root, "level1", "level2", "deep"))

	repos, err := ScanRepositories(context.Background(), []string{root}, 1)
	if err != nil {
		t.Fatalf("ScanRepositories: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("depth 1 should not find a repo nested 3 levels deep, got %+v", repos)
	}

	repos, err = ScanRepositories(context.Background(), []string{root}, 3)
	if err != nil {
		t.Fatalf("ScanRepositories: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("depth 3 should find the nested repo, got %+v", repos)
	}
}

func TestScanRepositoriesRootIsRepo(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root)

	repos, err := ScanRepositories(context.Background(), []string{root}, 1)
	if err != nil {
		t.Fatalf("ScanRepositories: %v", err)
	}
	if len(repos) != 1 || repos[0].Depth != 0 {
		t.Errorf("root repo should be found at depth 0, got %+v", repos)
	}
}

func TestScanRepositoriesSkipsMissingRoot(t *testing.T) {
	repos, err := ScanRepositories(context.Background(), []string{filepath.Join(t.TempDir(), "does-not-exist")}, 2)
	if err != nil {
		t.Fatalf("missing root should be skipped without error, got %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected no repos, got %+v", repos)
	}
}

func TestScanRepositoriesDoesNotDescendIntoRepos(t *testing.T) {
	root := t.TempDir()
	// A repo containing a nested repo must not report the nested one.
	makeRepo(t, filepath.Join(root, "outer"))
	makeRepo(t, filepath.Join(root, "outer", "inner"))

	repos, err := ScanRepositories(context.Background(), []string{root}, 3)
	if err != nil {
		t.Fatalf("ScanRepositories: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("scanner must not descend into repositories, got %+v", repos)
	}
}

// A cancelled scan (client disconnect, desktop stop button) must abort and
// report the cancellation rather than returning a partial walk that looks like
// a complete result.
func TestScanRepositoriesHonoursCancellation(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, filepath.Join(root, "alpha"))
	makeRepo(t, filepath.Join(root, "org", "beta"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	repos, err := ScanRepositories(ctx, []string{root}, 3)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scan should report context.Canceled, got %v (repos %+v)", err, repos)
	}
}

// Reaching MaxEntries keeps the repos already found on that root instead of
// dropping the entire root's results (the pre-fix behavior).
func TestScanRepositoriesMaxEntriesKeepsFound(t *testing.T) {
	root := t.TempDir()
	// Exceed MaxEntries with non-repo directories after one real repo. The
	// filler is named "z-…" so lexical order visits the repo first, the way
	// a real scan would find earlier entries before hitting the cap.
	makeRepo(t, filepath.Join(root, "real"))
	for i := 0; i < MaxEntries+5; i++ {
		if err := os.MkdirAll(filepath.Join(root, "z-filler", strconv.Itoa(i)), 0750); err != nil {
			t.Fatal(err)
		}
	}

	repos, err := ScanRepositories(context.Background(), []string{root}, 2)
	if err != nil {
		t.Fatalf("ScanRepositories: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("MaxEntries should keep the repos found, got %+v", repos)
	}
}

// cancelAfterN is a context whose Err() reports context.Canceled from the
// Nth call on. It makes mid-walk cancellation deterministic: WalkDir never
// consults Done(), and ScanRepositories polls Err() once per walked entry,
// so counting calls pins the exact callback where the cancellation lands.
type cancelAfterN struct {
	context.Context
	remaining int
}

func (c *cancelAfterN) Err() error {
	if c.remaining <= 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

// A cancellation that lands DURING a single root's walk must surface as an
// error. The pre-fix callback returned filepath.SkipAll, which WalkDir
// silently converts to nil, so the partial walk came back looking like a
// complete scan — and the stale-data cleanup then deleted every repository
// the walk never reached.
func TestScanRepositoriesMidWalkCancellationReturnsError(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, filepath.Join(root, "alpha"))
	makeRepo(t, filepath.Join(root, "beta"))
	// Land the cancellation after the walk has processed a few entries but
	// before it finishes the root.
	ctx := &cancelAfterN{Context: context.Background(), remaining: 3}

	repos, err := ScanRepositories(ctx, []string{root}, 3)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-walk cancellation must return context.Canceled, got %v (repos %+v)", err, repos)
	}
}

// A symlinked scan root used to end the walk after one entry (WalkDir Lstats
// the root, a symlink is not a directory) and silently found zero repos.
// Resolving the root itself must make discovery work again; the root-repo
// check already followed symlinks via os.Stat, so the two paths now agree.
func TestScanRepositoriesFollowsSymlinkedRoot(t *testing.T) {
	realRoot := t.TempDir()
	makeRepo(t, filepath.Join(realRoot, "alpha"))
	makeRepo(t, filepath.Join(realRoot, "beta"))

	linkRoot := filepath.Join(t.TempDir(), "dev-link")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("cannot create symlinks on this platform: %v", err)
	}

	repos, err := ScanRepositories(context.Background(), []string{linkRoot}, 2)
	if err != nil {
		t.Fatalf("ScanRepositories: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("symlinked root should discover 2 repos, got %+v", repos)
	}
}
