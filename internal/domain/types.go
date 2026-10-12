// Package domain defines the plain data types shared across RepoNest layers:
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

// PageVersion represents a historical snapshot of a wiki page (ADR-0021).
// Mirrors NoteVersion: full snapshot, 50 kept per page, FK cascade.
type PageVersion struct {
	ID        int64  `json:"id"`
	PageID    int64  `json:"page_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
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
