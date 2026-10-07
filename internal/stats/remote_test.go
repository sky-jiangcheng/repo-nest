package stats

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// RemoteWebURL drives a repo row's "open commit history" link, so the shape
// rules matter: a wrong link sends the user to a 404, which is worse than no
// link at all. These cases pin the accepted forms and the rejections.
func TestRemoteWebURL(t *testing.T) {
	newRepo := func(t *testing.T, remote string) string {
		t.Helper()
		dir := t.TempDir()
		for _, args := range [][]string{{"init"}, {"remote", "add", "origin", remote}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Skipf("git unavailable: %v (%s)", err, out)
			}
		}
		return dir
	}

	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{"gitlab ssh-less http", "http://gitlab.cbi.com/pay/pay-account.git", "http://gitlab.cbi.com/pay/pay-account/-/commits"},
		{"github https", "https://github.com/o/r.git", "https://github.com/o/r/-/commits"},
		{"https without .git", "https://github.com/o/r", "https://github.com/o/r/-/commits"},
		{"https trailing slash", "https://github.com/o/r/", "https://github.com/o/r/-/commits"},
		// SSH remotes are not rewritten: which forge, and which path prefix
		// its commit log lives at, cannot be inferred from git@host:group/repo.
		{"scp-style ssh", "git@github.com:o/r.git", ""},
		{"ssh protocol", "ssh://git@gitlab.cbi.com/pay/pay-account.git", ""},
		{"file remote", "file:///srv/git/pay-account.git", ""},
		// A host with no project path has nothing to link to.
		{"bare host", "https://gitlab.cbi.com", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RemoteWebURL(newRepo(t, tc.remote))
			if got != tc.want {
				t.Errorf("RemoteWebURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A repo with no origin configured (or a path that is not a repo at all) must
// yield "" so the UI can omit the link rather than render a dead one.
func TestRemoteWebURL_NoRemote(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %v (%s)", err, out)
	}
	if got := RemoteWebURL(dir); got != "" {
		t.Errorf("repo without origin: got %q, want \"\"", got)
	}

	notARepo := t.TempDir()
	if got := RemoteWebURL(notARepo); got != "" {
		t.Errorf("non-repo directory: got %q, want \"\"", got)
	}
	if got := RemoteWebURL(""); got != "" {
		t.Errorf("empty path: got %q, want \"\"", got)
	}
	if got := RemoteWebURL(filepath.Join(dir, "does-not-exist")); got != "" {
		t.Errorf("missing path: got %q, want \"\"", got)
	}
}

// The command must not leave a git process hanging or write to stdout when the
// path is not a repository — `git config` exits non-zero there, and the helper
// has to swallow that quietly.
func TestRemoteWebURL_NonRepoIsQuiet(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "not-a-repo.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := RemoteWebURL(dir); got != "" {
		t.Errorf("got %q, want \"\"", got)
	}
}
