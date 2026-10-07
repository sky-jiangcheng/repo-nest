package stats

import (
	"context"
	"os/exec"
	"strings"
)

// RemoteWebURL returns the browsable web URL for a repository's origin remote
// (e.g. "http://gitlab.cbi.com/pay/pay-account" for
// "http://gitlab.cbi.com/pay/pay-account.git"), or "" when the repo has no
// usable origin.
//
// It exists so a repo row can link to the actual commit history instead of
// listing per-author line counts, which are aggregate numbers the reader
// cannot act on. Recognised forges get their commit log path appended;
// anything else returns "" rather than a guessed URL — a wrong link is worse
// than no link.
//
// The trailing ".git" is stripped but nothing else is: SSH remotes
// (git@host:group/repo.git) are not rewritten, because guessing which forge
// and which path prefix a self-hosted server uses would produce broken links.
func RemoteWebURL(repoPath string) string {
	if repoPath == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), QueryTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "config", "--get", "remote.origin.url")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	remote := strings.TrimSpace(string(out))
	if remote == "" {
		return ""
	}
	remote = strings.TrimSuffix(remote, ".git")
	remote = strings.TrimSuffix(remote, "/")

	// Only https/ssh-free http remotes map to a predictable web URL. Anything
	// else (ssh://, git://, file://) is left to the user.
	if !strings.HasPrefix(remote, "http://") && !strings.HasPrefix(remote, "https://") {
		return ""
	}
	// A bare host (no path) has no project to link to.
	slash := strings.Index(remote, "://")
	if slash < 0 || !strings.Contains(remote[slash+3:], "/") {
		return ""
	}
	return remote + "/-/commits"
}
