package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
)

// Filesystem-path resolution (workspace → project / repository).
//
// Why this lives in internal/db rather than in the caller: the resolution needs
// both tables (repositories for the fine-grained match, projects for the coarse
// one), and internal/db is where every reader of those tables converges. The
// consumer is not just the VS Code extension — an MCP agent's cwd is the same
// kind of input, and having two implementations of "which project is this
// folder" is how the extension and the agent end up disagreeing about which
// knowledge a session gets.
//
// Two correctness points that are easy to get wrong and that the tests pin:
//
//  1. An ancestor test is NOT strings.HasPrefix. `/a/bc` starts with `/a/b`, but
//     `/a/b` is not a directory above `/a/bc`. Every ancestor check here goes
//     through isPathAncestor, which requires the boundary to be a separator.
//  2. Longest match wins, and that ordering is what makes the auto-grouped
//     containers survivable. This database has a project rooted at
//     `/Users/<user>` (an auto-grouped catch-all); plain "some project's root
//     contains my path" would claim every folder under home for it and inject the
//     wrong project's knowledge in silence.

// PathMatchHow describes how a path resolution succeeded, so the caller can tell
// the user what it matched on instead of asserting a guess.
const (
	// PathMatchExact: a stored path equals the queried directory.
	PathMatchExact = "exact"
	// PathMatchAncestor: a stored path is a directory above the queried one.
	PathMatchAncestor = "ancestor"
	// PathMatchCaseFolded: matched only after case folding (macOS/Windows are
	// case-insensitive by default; Linux is not, so this is a fallback, not a rule).
	PathMatchCaseFolded = "case-folded"
)

// NormalizeMatchPath cleans a path for comparison: absolute, no trailing
// separator, no `.`/`..` churn. An empty input stays empty so callers can
// distinguish "not provided" from "root".
func NormalizeMatchPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if p != string(filepath.Separator) {
		p = strings.TrimRight(p, string(filepath.Separator))
	}
	return p
}

// isPathAncestor reports whether parent is a directory strictly above child.
// The separator check is the whole point: HasPrefix("/a/bc", "/a/b") is true and
// would be a wrong answer.
func isPathAncestor(parent, child string) bool {
	if parent == "" || child == "" || parent == child {
		return false
	}
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}

// matchPath compares two paths, preferring an exact match and falling back to
// case folding. It reports which of the three relations held.
func matchPath(stored, queried string) (relation string, ok bool) {
	s, q := NormalizeMatchPath(stored), NormalizeMatchPath(queried)
	if s == "" || q == "" {
		return "", false
	}
	switch {
	case s == q:
		return PathMatchExact, true
	case strings.EqualFold(s, q):
		return PathMatchCaseFolded, true
	case isPathAncestor(s, q):
		return PathMatchAncestor, true
	case isPathAncestor(strings.ToLower(s), strings.ToLower(q)):
		return PathMatchAncestor + "+" + PathMatchCaseFolded, true
	}
	return "", false
}

// longerPath reports whether a is a deeper path than b — the tie-break that makes
// the most specific repository or project win.
func longerPath(a, b string) bool {
	return strings.Count(NormalizeMatchPath(a), string(filepath.Separator)) >
		strings.Count(NormalizeMatchPath(b), string(filepath.Separator))
}

// FindRepositoryByPath resolves a directory to the repository it belongs to:
// either the repository stored at exactly that path, or the deepest repository
// path that is an ancestor of it (opening a module inside a repo is the common
// case).
//
// Repositories with no project are returned anyway — the caller decides what to
// do with them — because "this folder is repo X" is a true fact independent of
// whether X was ever collected under a project.
func FindRepositoryByPath(database *sql.DB, dir string) (*Repository, string, error) {
	repos, err := GetAllRepositories(database)
	if err != nil {
		return nil, "", err
	}
	return PickRepositoryForPath(repos, dir)
}

// PickRepositoryForPath is the pure half of FindRepositoryByPath: given the
// repositories already loaded, resolve a directory. Exported so the service layer
// resolves with the SAME rules after loading the rows once — a second copy of this
// matching in another package is how the two eventually disagree.
func PickRepositoryForPath(repos []Repository, dir string) (*Repository, string, error) {
	queried := NormalizeMatchPath(dir)
	if queried == "" {
		return nil, "", nil
	}
	var best *Repository
	var bestHow string
	for i := range repos {
		r := &repos[i]
		how, ok := matchPath(r.Path, queried)
		if !ok {
			continue
		}
		// exact/case-folded beats a mere ancestor, and among equals the deeper
		// path wins.
		if best == nil {
			best, bestHow = r, how
			continue
		}
		bestExact := bestHow == PathMatchExact || bestHow == PathMatchCaseFolded
		thisExact := how == PathMatchExact || how == PathMatchCaseFolded
		switch {
		case thisExact && !bestExact:
			best, bestHow = r, how
		case thisExact == bestExact && longerPath(r.Path, best.Path):
			best, bestHow = r, how
		}
	}
	if best == nil {
		return nil, "", nil
	}
	return best, bestHow, nil
}

// FindProjectByPath resolves a directory to the project that owns it.
//
// `allowCatchAll` controls the second half of the rule and is the guard against
// the failure this database can actually produce: project rows with
// is_auto_grouped = 1 are scan-time *containers* (one was rooted at the user's
// home directory), so treating them as ancestors claims every folder under home
// and hands the session the wrong project's knowledge with no signal that
// anything was guessed. With allowCatchAll false, an auto-grouped project can
// still match EXACTLY — opening that folder deliberately is a real answer — it
// just cannot win by being an ancestor.
func FindProjectByPath(database *sql.DB, dir string, allowCatchAll bool) (*Project, string, error) {
	projects, err := GetAllProjects(database)
	if err != nil {
		return nil, "", err
	}
	return PickProjectForPath(projects, dir, allowCatchAll)
}

// PickProjectForPath is the pure half of FindProjectByPath; see
// PickRepositoryForPath for why it is exported.
func PickProjectForPath(projects []Project, dir string, allowCatchAll bool) (*Project, string, error) {
	queried := NormalizeMatchPath(dir)
	if queried == "" {
		return nil, "", nil
	}
	var best *Project
	var bestHow string
	for i := range projects {
		p := &projects[i]
		how, ok := matchPath(p.RootPath, queried)
		if !ok {
			continue
		}
		isAncestor := strings.HasPrefix(how, PathMatchAncestor)
		if isAncestor && p.IsAutoGrouped && !allowCatchAll {
			continue // catch-all container: refuses to adopt a subdirectory
		}
		if best == nil {
			best, bestHow = p, how
			continue
		}
		bestAncestor := strings.HasPrefix(bestHow, PathMatchAncestor)
		thisAncestor := isAncestor
		switch {
		case bestAncestor && !thisAncestor:
			best, bestHow = p, how
		case bestAncestor == thisAncestor && longerPath(p.RootPath, best.RootPath):
			best, bestHow = p, how
		}
	}
	if best == nil {
		return nil, "", nil
	}
	return best, bestHow, nil
}

// AttachNoteToRepositoryByID is the writer note_repositories never had: record
// that a note came from work inside a specific repository. Idempotent (the PK
// absorbs a repeat), and it refuses a note/repository pair whose repository does
// not exist rather than writing an orphan the UI can never show.
func AttachNoteToRepositoryByID(database *sql.DB, noteID, repositoryID int64) error {
	var one int
	err := database.QueryRow("SELECT 1 FROM repositories WHERE id = ?", repositoryID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRepositoryNotFound
	}
	if err != nil {
		return err
	}
	return AttachNoteToRepository(database, noteID, repositoryID)
}

// ErrRepositoryNotFound distinguishes "the repository row is gone" from a
// storage failure, so a caller can degrade to project scope instead of failing a
// handoff that already produced a note.
var ErrRepositoryNotFound = errors.New("repository not found")

// The scoped reads over this table already exist (RepositoriesForNote and
// NotesForRepository in wiki.go); no aliases are added here, because a second
// name for the same query is exactly the kind of surface that drifts.
