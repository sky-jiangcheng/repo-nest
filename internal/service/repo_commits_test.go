package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// GetRepoCommits backs the repo row's commit list. The database cannot answer
// it: daily_stats holds "who changed how many lines on which day" with no
// message and no SHA, so the list has to come from git.
func TestGetRepoCommits_ReadsGitAndCarriesHash(t *testing.T) {
	svc, _ := setupService(t)
	dir := newGitRepoWithCommits(t)
	pid := seedProject(t, svc.db, "p", filepath.Dir(dir))
	rid := seedRepo(t, svc.db, dir, pid)

	commits, err := svc.GetRepoCommits(rid, 10)
	if err != nil {
		t.Fatalf("GetRepoCommits: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("want 3 commits, got %d", len(commits))
	}
	// Newest first.
	if commits[0].Message != "third" {
		t.Errorf("commits[0].Message = %q, want %q (newest first)", commits[0].Message, "third")
	}
	// The hash is what makes a row actionable (cherry-pick / forge search), so
	// it must survive the parser rather than being dropped on the floor.
	for i, c := range commits {
		if len(c.Hash) != 40 {
			t.Errorf("commits[%d].Hash = %q, want a 40-char SHA", i, c.Hash)
		}
	}
	if commits[0].Author == "" {
		t.Error("commits[0].Author is empty")
	}
}

func TestGetRepoCommits_EmptyRepoReturnsEmptySlice(t *testing.T) {
	svc, _ := setupService(t)
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %v (%s)", err, out)
	}
	pid := seedProject(t, svc.db, "p", filepath.Dir(dir))
	rid := seedRepo(t, svc.db, dir, pid)

	commits, err := svc.GetRepoCommits(rid, 10)
	if err != nil {
		t.Fatalf("GetRepoCommits: %v", err)
	}
	// Non-nil, so the JSON contract is an array and clients can read .length.
	if commits == nil {
		t.Fatal("commits = nil, want an empty slice (JSON null breaks .length)")
	}
	if len(commits) != 0 {
		t.Errorf("want 0 commits, got %d", len(commits))
	}
}

func TestGetRepoCommits_UnknownRepoIsDistinguishable(t *testing.T) {
	svc, _ := setupService(t)
	if _, err := svc.GetRepoCommits(9999, 10); err == nil {
		t.Fatal("expected an error for a repo id that does not exist")
	} else if err != ErrRepoNotFound {
		t.Errorf("got %v, want ErrRepoNotFound so the UI can say 'gone' not 'broken'", err)
	}
}

// The limit is caller-supplied; an absurd value must be clamped rather than
// turned into a `git log -999999` that walks the whole history.
func TestGetRepoCommits_ClampsLimit(t *testing.T) {
	svc, _ := setupService(t)
	dir := newGitRepoWithCommits(t)
	pid := seedProject(t, svc.db, "p", filepath.Dir(dir))
	rid := seedRepo(t, svc.db, dir, pid)

	for _, limit := range []int{0, -5, 100000} {
		commits, err := svc.GetRepoCommits(rid, limit)
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if len(commits) > 200 {
			t.Errorf("limit %d: got %d commits, want the clamp to hold at <=200", limit, len(commits))
		}
	}
}

// newGitRepoWithCommits builds a throwaway repo with three ordered commits.
func newGitRepoWithCommits(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init")
	for _, msg := range []string{"first", "second", "third"} {
		if err := os.WriteFile(filepath.Join(dir, msg+".txt"), []byte(msg), 0o600); err != nil {
			t.Fatal(err)
		}
		run("add", ".")
		run("commit", "-m", msg)
	}
	return dir
}

