package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ScanRootRejection explains why one submitted scan root was refused. The UI
// shows these verbatim, so Reason is a stable machine-readable code and Detail
// is the human-facing sentence.
type ScanRootRejection struct {
	// Path is the submitted value, cleaned for display but otherwise untouched.
	Path string `json:"path"`
	// Reason is a stable code: empty | relative | not_found | not_a_directory
	// | duplicate | inaccessible.
	Reason string `json:"reason"`
	// Detail is a short human-readable explanation.
	Detail string `json:"detail"`
}

// ScanRootsResult is what UpdateScanRoots persists and reports back.
//
// Roots is the normalised list actually stored — it may differ from what was
// submitted (trailing slashes removed, duplicates collapsed). Rejected lists the
// entries that were dropped, so the caller learns the outcome instead of having
// to re-read the config to discover what was lost.
type ScanRootsResult struct {
	ScanRoots []string            `json:"scan_roots"`
	Rejected  []ScanRootRejection `json:"rejected"`
}

// normalizeScanRoots canonicalises a user-supplied scan-root list and splits it
// into the roots worth storing and the entries worth refusing.
//
// Why normalisation instead of raw storage: the settings UI appends a new root
// to the existing list and previously did no dedup, so "/home/u" and
// "/home/u/" were stored as two entries pointing at one directory. The list is
// rendered with the path as its React key, so duplicates also produced a
// duplicate-key warning. filepath.Clean collapses both spellings to one, and
// the same pass enforces that every stored root is a real directory.
//
// Deliberate omissions:
//   - Symlinks are NOT resolved (EvalSymlinks). The user typed a path; silently
//     rewriting it to its target would be surprising and, on macOS where
//     /tmp is a symlink to /private/tmp, would also rewrite our own test paths.
//   - Validation is per-entry, not whole-list. Windows seeds every drive letter
//     as a scan root, so an absent or unmounted drive is a legitimate stored
//     state. Rejecting the whole list would make it impossible to remove any
//     other root while one dead drive remained, trading one bug for a worse one.
func normalizeScanRoots(submitted []string) (roots []string, rejected []ScanRootRejection) {
	// dedupe keys on a case-folded form: Windows and macOS paths are
	// case-insensitive, so "C:\Repo" and "c:\repo" are one directory, and storing
	// both would again duplicate the scan.
	seen := make(map[string]bool, len(submitted))
	for _, raw := range submitted {
		input := strings.TrimSpace(raw)
		if input == "" {
			rejected = append(rejected, ScanRootRejection{
				Path:   raw,
				Reason: "empty",
				Detail: "path is empty",
			})
			continue
		}

		// Expand a leading ~ so "~/Desktop" works like it does in a shell. This
		// is friendlier than rejecting it, and the expanded result is what gets
		// stored, so it is also what the user sees in the list afterwards.
		if input == "~" || strings.HasPrefix(input, "~/") || strings.HasPrefix(input, "~"+string(os.PathSeparator)) {
			if home, err := os.UserHomeDir(); err == nil && home != "" {
				if input == "~" {
					input = home
				} else {
					// Trim the separator the user actually typed: "~/" on any
					// platform, and "~\" on Windows. Leaving it in would make
					// filepath.Join treat the remainder as rooted and can produce
					// a doubled separator before Clean.
					input = filepath.Join(home, strings.TrimLeft(input[1:], `/\`))
				}
			}
		}

		if !filepath.IsAbs(input) {
			rejected = append(rejected, ScanRootRejection{
				Path:   input,
				Reason: "relative",
				Detail: "must be an absolute path",
			})
			continue
		}

		// Clean collapses trailing slashes, "./" segments and "x/.." pairs, so
		// all spellings of one directory converge on a single stored string.
		cleaned := filepath.Clean(input)
		if seen[dedupeKey(cleaned)] {
			rejected = append(rejected, ScanRootRejection{
				Path:   cleaned,
				Reason: "duplicate",
				Detail: "already in the list",
			})
			continue
		}

		info, err := os.Stat(cleaned)
		switch {
		case err != nil && os.IsNotExist(err):
			rejected = append(rejected, ScanRootRejection{
				Path:   cleaned,
				Reason: "not_found",
				Detail: "directory does not exist",
			})
			continue
		case err != nil:
			rejected = append(rejected, ScanRootRejection{
				Path:   cleaned,
				Reason: "inaccessible",
				Detail: "cannot access directory",
			})
			continue
		case !info.IsDir():
			rejected = append(rejected, ScanRootRejection{
				Path:   cleaned,
				Reason: "not_a_directory",
				Detail: "path is a file, not a directory",
			})
			continue
		}

		seen[dedupeKey(cleaned)] = true
		roots = append(roots, cleaned)
	}
	return roots, rejected
}

// dedupeKey folds case on platforms whose filesystems normally ignore it, so
// "C:\Repo" and "c:\repo" collapse to one entry. Linux is excluded on purpose:
// its filesystem is case-sensitive by default, so folding there would silently
// discard a genuinely distinct directory.
func dedupeKey(path string) string {
	switch runtime.GOOS {
	case "windows", "darwin":
		return strings.ToLower(path)
	default:
		return path
	}
}
