package stats

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initGitRepo creates a scratch repository with one commit per entry
// (message -> author). Mirrors the fixture in internal/knowledge.
func initGitRepoForCommits(t *testing.T, commits map[string]string) string {
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

// Regression: the field separator was passed to git as a LITERAL NUL byte
// inside the --pretty=format: argument. POSIX argv cannot contain NUL, so
// execve failed for every repository, the error was swallowed per-repo, and
// GetRecentCommits returned an empty list for every project — the project
// detail page never showed a single commit despite healthy repositories.
// The separator must reach git as the %x00 format specifier instead.
func TestGetRecentCommitsRealRepo(t *testing.T) {
	root := initGitRepoForCommits(t, map[string]string{
		"fix: alpha": "alice",
		"fix: beta":  "bob",
	})

	all, err := GetRecentCommits([]string{root}, "", 10)
	if err != nil {
		t.Fatalf("GetRecentCommits: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 commits, got %d (%+v)", len(all), all)
	}
	gotMsgs := map[string]bool{all[0].Message: true, all[1].Message: true}
	if !gotMsgs["fix: alpha"] || !gotMsgs["fix: beta"] {
		t.Errorf("messages not parsed intact: %+v", all)
	}
	for _, c := range all {
		if c.Author != "alice" && c.Author != "bob" {
			t.Errorf("unexpected author %q in %+v", c.Author, c)
		}
	}

	alice, err := GetRecentCommits([]string{root}, "alice", 10)
	if err != nil {
		t.Fatalf("GetRecentCommits(author filter): %v", err)
	}
	if len(alice) != 1 || alice[0].Author != "alice" || alice[0].Message != "fix: alpha" {
		t.Errorf("author filter returned %+v, want one alice commit", alice)
	}
}
