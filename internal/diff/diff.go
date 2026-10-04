// Package diff implements the line-based diff used to compare note versions.
package diff

import "strings"

// maxDPCells bounds the LCS dp table. Note content is capped at 100 KB, but
// that bounds bytes, not lines: 100 KB of empty lines is ~100k lines per side,
// and an unbounded (m+1)*(n+1) int32 table would ask for ~40 GB and
// fatal-OOM the process (a runtime out-of-memory is not recoverable). 16M
// cells is 64 MB — enough for any real note diff (4k x 4k lines) — and beyond
// it the diff degrades to "everything removed, everything added": lossy, but
// honest and cheap.
const maxDPCells = 16_000_000

// Lines produces a simple line-based diff between old and new content using
// the longest common subsequence. Output lines are prefixed with ' ' (context),
// '-' (removal) or '+' (addition).
func Lines(old, new string) string {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	// Trim the common prefix/suffix first: a one-line edit in a long note
	// then runs a tiny dp table instead of the full m*n one. The trimmed
	// lines are common by construction, so emitting them back as context
	// around the core diff reproduces the untrimmed output.
	var prefix, suffix []string
	for len(oldLines) > 0 && len(newLines) > 0 && oldLines[0] == newLines[0] {
		prefix = append(prefix, oldLines[0])
		oldLines, newLines = oldLines[1:], newLines[1:]
	}
	for len(oldLines) > 0 && len(newLines) > 0 && oldLines[len(oldLines)-1] == newLines[len(newLines)-1] {
		suffix = append([]string{oldLines[len(oldLines)-1]}, suffix...)
		oldLines, newLines = oldLines[:len(oldLines)-1], newLines[:len(newLines)-1]
	}

	m, n := len(oldLines), len(newLines)

	var out []string
	for _, l := range prefix {
		out = append(out, " "+l)
	}

	switch {
	case m > 0 && n > 0 && (m+1)*(n+1) > maxDPCells:
		// Over budget: the sequences share no trimmable edges AND are too
		// large to diff exactly. Degraded diff: everything removed, then
		// everything added.
		for _, l := range oldLines {
			out = append(out, "-"+l)
		}
		for _, l := range newLines {
			out = append(out, "+"+l)
		}
	default:
		out = append(out, lcsDiff(oldLines, newLines)...)
	}

	for _, l := range suffix {
		out = append(out, " "+l)
	}
	return strings.Join(out, "\n")
}

// lcsDiff diffs the two line slices with a full LCS dp table. m, n must
// satisfy (m+1)*(n+1) <= maxDPCells — enforced by Lines.
func lcsDiff(oldLines, newLines []string) []string {
	m, n := len(oldLines), len(newLines)

	// Build LCS table.
	dp := make([][]int32, m+1)
	for i := range dp {
		dp[i] = make([]int32, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] > dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	// Backtrack to produce the diff.
	var result []string
	i, j := 0, 0
	for i < m || j < n {
		switch {
		case i < m && j < n && oldLines[i] == newLines[j]:
			result = append(result, " "+oldLines[i])
			i++
			j++
		case j < n && (i == m || dp[i][j+1] >= dp[i+1][j]):
			result = append(result, "+"+newLines[j])
			j++
		default:
			result = append(result, "-"+oldLines[i])
			i++
		}
	}
	return result
}
