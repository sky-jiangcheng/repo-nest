package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// RepoInfo holds information about a discovered git repository.
type RepoInfo struct {
	Path  string // absolute path to the repository
	Depth int    // depth from scan root
}

// MaxEntries is the maximum number of directories to scan before stopping.
const MaxEntries = 10000

// ScanRepositories recursively searches for .git directories within the given roots,
// up to the specified max depth. Returns a list of discovered repositories.
// The walk honours ctx: a cancelled scan (agent client disconnect, desktop
// "stop" button) aborts promptly instead of finishing a multi-minute walk
// for nobody — the returned error is then ctx.Err(), never a partial result
// presented as a complete one.
func ScanRepositories(ctx context.Context, roots []string, maxDepth int) ([]RepoInfo, error) {
	var repos []RepoInfo
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return repos, err
		}
		found, err := scanRoot(ctx, root, maxDepth)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return repos, ctxErr
			}
			// skip inaccessible roots
			continue
		}
		repos = append(repos, found...)
	}
	return repos, nil
}

// scanRoot scans a single root directory for git repositories.
func scanRoot(ctx context.Context, root string, maxDepth int) ([]RepoInfo, error) {
	var repos []RepoInfo
	entriesCount := 0

	// A symlinked scan root (~/dev -> /Volumes/Big/dev is a common layout)
	// ended the walk after one entry: WalkDir Lstats the root and a symlink
	// is not a directory, so the root silently yielded zero repos. Resolve
	// the root itself only — symlinks INSIDE the tree are still not followed
	// (loop protection, unchanged) — and walk the resolved path so the depth
	// accounting and discovered repo paths are consistent with what is
	// actually traversed.
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != root {
		root = resolved
	}

	// Check if root itself is a git repo.
	// KNOWN TRADE-OFF: only a .git DIRECTORY counts. Worktrees and submodules
	// mark their checkout with a .git FILE ("gitdir: <path>") and are not
	// recognised as repos — deliberate for now, because treating them as
	// repos changes user-visible data (submodule checkouts would surface as
	// separate repos in the knowledge base). To support them, parse the
	// gitdir: pointer here and in the walk below.
	rootGitDir := filepath.Join(root, ".git")
	if info, err := os.Stat(rootGitDir); err == nil && info.IsDir() {
		absPath, _ := filepath.Abs(root)
		repos = append(repos, RepoInfo{
			Path:  absPath,
			Depth: 0,
		})
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// permission denied, skip this entry
			if os.IsPermission(err) {
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return nil
		}

		// Cancelled mid-walk: propagate ctx.Err() verbatim. filepath.SkipAll
		// would NOT work here — WalkDir converts it to a nil error, and a nil
		// return would present the partial walk as a complete scan (the stale
		// data cleanup would then delete everything the walk never reached).
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		if !d.IsDir() {
			return nil
		}

		// Calculate depth relative to root
		relPath, _ := filepath.Rel(root, path)
		if relPath == "." {
			return nil
		}

		depth := len(strings.Split(filepath.ToSlash(relPath), "/"))

		// Stop if exceeding max depth
		if depth > maxDepth {
			return filepath.SkipDir
		}

		entriesCount++
		if entriesCount > MaxEntries {
			return filepath.SkipAll
		}

		// Check for .git directory
		gitDir := filepath.Join(path, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			absPath, _ := filepath.Abs(path)
			repos = append(repos, RepoInfo{
				Path:  absPath,
				Depth: depth,
			})
			// Don't descend into git repositories
			return filepath.SkipDir
		}

		return nil
	})

	if err == nil {
		// Cancellation can land between the last callback and this return; a
		// partial walk must never masquerade as a complete one.
		err = ctx.Err()
	}
	if err != nil {
		return repos, err
	}
	// MaxEntries (SkipAll) and root-repo paths above end here: WalkDir turns
	// SkipAll into a nil error, so the repos already found on this root are
	// kept and reported as the root's result.
	return repos, nil
}
