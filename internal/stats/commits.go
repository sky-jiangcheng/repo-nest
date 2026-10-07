package stats

import (
	"context"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RecentCommit holds information about the most recent commit.
type RecentCommit struct {
	// Hash is the full commit SHA. Empty only if git output was unparseable.
	// Kept because a commit row is actionable: it is what you copy to cherry-pick
	// or paste into a search, and what identifies the commit on the forge.
	Hash    string `json:"hash"`
	Time    string `json:"time"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Repo    string `json:"repo"`
	Branch  string `json:"branch"`
}

// GetRecentCommit queries the most recent commit across all repositories.
func GetRecentCommit(repoPaths []string, filterAuthor string) (*RecentCommit, error) {
	if err := ValidateAuthor(filterAuthor); err != nil {
		return nil, err
	}
	var best *RecentCommit

	for _, repoPath := range repoPaths {
		args := []string{
			"log", "-1",
			"--pretty=format:%H%n%an%n%at%n%s%n%D",
		}
		if filterAuthor != "" {
			args = append(args, "--author="+filterAuthor)
		}

		ctx, cancel := context.WithTimeout(context.Background(), QueryTimeout)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoPath
		out, err := cmd.Output()
		cancel()
		if err != nil {
			continue
		}

		lines := strings.Split(string(out), "\n")
		if len(lines) < 4 {
			continue
		}

		ts, _ := strconv.ParseInt(lines[2], 10, 64)
		if ts == 0 {
			continue
		}

		branch := ""
		if len(lines) >= 5 {
			branch = extractBranch(lines[4])
		}

		commitTime := time.Unix(ts, 0)
		if best == nil || commitTime.After(time.Unix(parseTimestamp(best.Time), 0)) {
			best = &RecentCommit{
				Hash:    lines[0],
				Time:    commitTime.Format("2006-01-02 15:04:05"),
				Author:  lines[1],
				Message: lines[3],
				Repo:    repoPath,
				Branch:  branch,
			}
		}
	}

	return best, nil
}

// GetRecentCommits returns the most recent commits across the given repositories,
// merged and sorted by time descending, capped at limit. The author filter is
// optional; an empty filter returns commits from all authors.
func GetRecentCommits(repoPaths []string, filterAuthor string, limit int) ([]RecentCommit, error) {
	if err := ValidateAuthor(filterAuthor); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}

	var all []RecentCommit
	// Field separator: NUL byte, which cannot appear inside git's text fields.
	const sep = "\x00"

	for _, repoPath := range repoPaths {
		args := []string{
			"log", "-" + strconv.Itoa(limit),
			// The NUL separator must reach git as the FORMAT SPECIFIER %x00 —
			// a literal NUL byte inside an argv string fails at execve (POSIX
			// argv is NUL-terminated), so every command errored and the commit
			// list was silently empty for every project. %n (as GetLastCommit
			// uses) cannot be used here: commit subjects may contain newlines.
			"--pretty=format:%H%x00%an%x00%at%x00%s%x00%D",
		}
		if filterAuthor != "" {
			args = append(args, "--author="+filterAuthor)
		}

		ctx, cancel := context.WithTimeout(context.Background(), QueryTimeout)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoPath
		out, err := cmd.Output()
		cancel()
		if err != nil {
			continue
		}

		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, sep, 5)
			if len(parts) < 4 {
				continue
			}
			ts, _ := strconv.ParseInt(parts[2], 10, 64)
			if ts == 0 {
				continue
			}
			branch := ""
			if len(parts) >= 5 {
				branch = extractBranch(parts[4])
			}
			all = append(all, RecentCommit{
				Hash:    parts[0],
				Time:    time.Unix(ts, 0).Format("2006-01-02 15:04:05"),
				Author:  parts[1],
				Message: parts[3],
				Repo:    repoPath,
				Branch:  branch,
			})
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return parseTimestamp(all[i].Time) > parseTimestamp(all[j].Time)
	})
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}
