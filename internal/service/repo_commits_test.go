package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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

// GetProjectCommits backs the commits tab's merged timeline.
func TestGetProjectCommits_MergesReposNewestFirst(t *testing.T) {
	svc, _ := setupService(t)
	older := newDatedGitRepo(t, "2026-01-01T10:00:00", []string{"old-a", "old-b"})
	newer := newDatedGitRepo(t, "2026-03-01T10:00:00", []string{"new-a", "new-b"})
	pid := seedProject(t, svc.db, "p", t.TempDir())
	seedRepo(t, svc.db, older, pid)
	seedRepo(t, svc.db, newer, pid)

	commits, err := svc.GetProjectCommits(pid, 10)
	if err != nil {
		t.Fatalf("GetProjectCommits: %v", err)
	}
	if len(commits) != 4 {
		t.Fatalf("want 4 commits merged from both repos, got %d", len(commits))
	}
	// The whole point of the merged view is that a repo cannot hog the top of
	// the list: ordering is by time across repos, so the newest repo's newest
	// commit leads and the oldest repo's oldest commit trails.
	if commits[0].Message != "new-b" || commits[0].Repo != newer {
		t.Errorf("commits[0] = %q in %q, want newest commit of the newer repo", commits[0].Message, commits[0].Repo)
	}
	if commits[len(commits)-1].Message != "old-a" || commits[len(commits)-1].Repo != older {
		t.Errorf("last commit = %q in %q, want oldest commit of the older repo", commits[len(commits)-1].Message, commits[len(commits)-1].Repo)
	}
	seen := map[string]int{}
	for _, c := range commits {
		seen[c.Repo]++
	}
	if seen[older] != 2 || seen[newer] != 2 {
		t.Errorf("per-repo counts = %v, want 2 from each repo", seen)
	}
	// The merged row labels its repo and links its SHA, so both must be present.
	for i, c := range commits {
		if c.Repo == "" || len(c.Hash) != 40 {
			t.Errorf("commits[%d]: repo=%q hash=%q, want both populated", i, c.Repo, c.Hash)
		}
	}
}

// A project with no repositories is a normal state (just created, or all repos
// removed), not an error: the timeline shows an empty list.
func TestGetProjectCommits_ProjectWithoutReposIsEmptyNotNil(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "empty", t.TempDir())

	commits, err := svc.GetProjectCommits(pid, 10)
	if err != nil {
		t.Fatalf("GetProjectCommits: %v, want an empty log rather than an error", err)
	}
	if commits == nil {
		t.Fatal("commits = nil, want an empty slice (JSON null breaks .length)")
	}
	if len(commits) != 0 {
		t.Errorf("want 0 commits, got %d", len(commits))
	}
}

func TestGetProjectCommits_UnknownProjectIsErrProjectNotFound(t *testing.T) {
	svc, _ := setupService(t)
	// GetRepositoriesByProjectID returns an empty slice (no error) for a missing
	// project, so without the explicit existence check a bogus id would be
	// indistinguishable from a repo-less project.
	if _, err := svc.GetProjectCommits(9999, 10); err != ErrProjectNotFound {
		t.Errorf("got %v, want ErrProjectNotFound so the UI can say 'gone' not 'broken'", err)
	}
}

// The limit is caller-supplied; an absurd value must be clamped rather than
// turned into a `git log -999999` per repository.
func TestGetProjectCommits_ClampsLimit(t *testing.T) {
	svc, _ := setupService(t)
	dir := newGitRepoWithCommits(t)
	pid := seedProject(t, svc.db, "p", filepath.Dir(dir))
	seedRepo(t, svc.db, dir, pid)

	for _, limit := range []int{0, -5, 100000} {
		commits, err := svc.GetProjectCommits(pid, limit)
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if len(commits) > 200 {
			t.Errorf("limit %d: got %d commits, want the clamp to hold at <=200", limit, len(commits))
		}
		if len(commits) == 0 {
			t.Errorf("limit %d: got 0 commits, want the fallback (50) to still return history", limit)
		}
	}
}

// newDatedGitRepo builds a throwaway repo whose commits carry fixed timestamps,
// so cross-repo merge ordering can be asserted without racing the clock.
func newDatedGitRepo(t *testing.T, firstDate string, msgs []string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(when string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=t@example.com",
			"GIT_AUTHOR_DATE="+when, "GIT_COMMITTER_DATE="+when,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run(firstDate, "init")
	// Each commit one day later than the last: "oldest first" in the argument
	// list, so the last message is the newest commit.
	for i, msg := range msgs {
		when := shiftDate(t, firstDate, i)
		if err := os.WriteFile(filepath.Join(dir, msg+".txt"), []byte(msg), 0o600); err != nil {
			t.Fatal(err)
		}
		run(when, "add", ".")
		run(when, "commit", "-m", msg)
	}
	return dir
}

func shiftDate(t *testing.T, iso string, days int) string {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	ts, err := time.ParseInLocation("2006-01-02T15:04:05", iso, loc)
	if err != nil {
		t.Fatalf("bad date %q: %v", iso, err)
	}
	return ts.AddDate(0, 0, days).Format("2006-01-02T15:04:05")
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
