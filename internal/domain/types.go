// Package domain defines the plain data types shared across Repotory layers:
// persistence (internal/db), business logic (internal/service) and the
// bindings exposed to the frontend, CLI and MCP server. The types carry no
// behaviour and no storage semantics so every layer can depend on them
// without cycles.
package domain

// Project represents a row in the projects table.
type Project struct {
	ID            int64  `db:"id" json:"id"`
	Name          string `db:"name" json:"name"`
	RootPath      string `db:"root_path" json:"root_path"`
	LevelOverride int    `db:"level_override" json:"level_override"`
	IsAutoGrouped bool   `db:"is_auto_grouped" json:"is_auto_grouped"`
	IsStarred     bool   `db:"is_starred" json:"is_starred"`
	Collected     bool   `db:"collected" json:"collected"`
	CollectedAt   string `db:"collected_at" json:"collected_at"`
	CreatedAt     string `db:"created_at" json:"created_at"`
	// ProjectKind classifies how the row came to exist — see the ProjectKind*
	// constants below (ADR-0023). Wired into projectColumns by migration v24.
	ProjectKind string `db:"project_kind" json:"project_kind"`
}

// ProjectKind values (ADR-0023). The set is validated in Go, not by a SQL CHECK,
// because adding a table-level CHECK to the existing projects table would force a
// full rebuild (see ADR-0021 implementation corrections).
const (
	// ProjectKindScanned is a project discovered by the repository scanner.
	// It is the default for every row that predates v24.
	ProjectKindScanned = "scanned"
	// ProjectKindImportPlaceholder is a landing project created during an
	// external-knowledge import when the note matched no existing project. Its
	// root_path is the source repo path, so a later scan of that repo promotes
	// it to ProjectKindScanned.
	ProjectKindImportPlaceholder = "import_placeholder"
	// ProjectKindDefault is the seeded "Uncategorized" fallback project that
	// receives imports with no derivable repo path, and the organize target of
	// last resort. It must never be deleted.
	ProjectKindDefault = "default"
)

// Seed values for the single ProjectKindDefault row (ADR-0023 decision 2).
// The root_path is a synthetic sentinel no real filesystem path can produce, so
// it satisfies projects.root_path NOT NULL UNIQUE without ever colliding with a
// scanned repository. The display name is localized by the frontend from the
// kind; this fixed value is only the stored/internal name.
const (
	DefaultProjectName     = "未分类"
	DefaultProjectRootPath = "repotory://memory/default"
)

// OrganizeGroup is one row in the knowledge-base "organize" surface (ADR-0023
// decision 5): a landing project — an import placeholder or the default
// ("Uncategorized") bucket — that holds notes the user has not yet assigned to a
// real, scanner-backed project. RepoBound reports whether a scanned repository
// currently points at the project; an unbound placeholder whose repo has vanished
// from disk is the "source repo gone" case where deleting it may also delete its
// notes (a manual, user-confirmed action — the default project is never offered).
type OrganizeGroup struct {
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	RootPath  string `json:"root_path"`
	Kind      string `json:"kind"`
	NoteCount int    `json:"note_count"`
	RepoBound bool   `json:"repo_bound"`
}

// Repository represents a row in the repositories table.
type Repository struct {
	ID          int64   `json:"id"`
	Path        string  `json:"path"`
	ProjectID   *int64  `json:"project_id"`
	LastScanned *string `json:"last_scanned_at"`
}

// Todo represents a row in the project_todos table.
type Todo struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
	Priority  int    `json:"priority"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Note represents a row in the project_notes table.
type Note struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Tags      string `json:"tags"`
	Kind      string `json:"kind"`
	Pinned    bool   `json:"pinned"`
	Source    string `json:"source"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// NoteWithProject is a note joined with its parent project.
type NoteWithProject struct {
	Note
	ProjectName string `json:"project_name"`
}

// NoteVersion represents a historical snapshot of a note.
type NoteVersion struct {
	ID        int64  `json:"id"`
	NoteID    int64  `json:"note_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Tags      string `json:"tags"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"`
}

// NoteDiff represents a line-level diff between two versions.
type NoteDiff struct {
	VersionID int64  `json:"version_id"`
	CreatedAt string `json:"created_at"`
	Diff      string `json:"diff"`
}

// TodoCount holds incomplete and total todo counts for a project.
type TodoCount struct {
	ProjectID int64 `json:"project_id"`
	Count     int   `json:"count"`
	Total     int   `json:"total"`
}

// NoteCount holds the note count for a project.
type NoteCount struct {
	ProjectID int64 `json:"project_id"`
	Count     int   `json:"count"`
}

// DailyStat represents a row in the daily_stats table.
type DailyStat struct {
	ID           int64  `json:"id"`
	RepositoryID int64  `json:"repository_id"`
	StatDate     string `json:"stat_date"`
	Author       string `json:"author"`
	FilesChanged int    `json:"files_changed"`
	LinesAdded   int    `json:"lines_added"`
	LinesDeleted int    `json:"lines_deleted"`
}

// HeatmapDay holds aggregated stats for a single day.
type HeatmapDay struct {
	Date         string `json:"date"`
	LinesAdded   int    `json:"lines_added"`
	LinesDeleted int    `json:"lines_deleted"`
	Commits      int    `json:"commits"`
}

// SearchHit is a unified search result from notes and todos.
type SearchHit struct {
	Type      string  `json:"type"`
	ID        int64   `json:"id"`
	ProjectID int64   `json:"project_id"`
	Title     string  `json:"title"`
	Snippet   string  `json:"snippet"`
	Rank      float64 `json:"rank,omitempty"` // bm25 score; lower = more relevant
}

// RepoMeta represents a row in the repo_meta table.
type RepoMeta struct {
	RepositoryID    int64  `json:"repository_id"`
	TechStack       string `json:"tech_stack"`
	ReadmeExcerpt   string `json:"readme_excerpt"`
	Languages       string `json:"languages"`
	Dependencies    string `json:"dependencies"`
	TopContributors string `json:"top_contributors"`
	Activity        string `json:"activity"`
	UpdatedAt       string `json:"updated_at"`
}
