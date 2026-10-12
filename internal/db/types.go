package db

import "repo-nest/internal/domain"

// Row types are defined in internal/domain so business layers (service,
// bindings, CLI, MCP) can share them without depending on the storage
// package. These aliases keep the historical db.X names working for callers
// inside and outside this package.
type (
	Project         = domain.Project
	Repository      = domain.Repository
	Todo            = domain.Todo
	Note            = domain.Note
	NoteWithProject = domain.NoteWithProject
	NoteVersion     = domain.NoteVersion
	NoteDiff        = domain.NoteDiff
	PageVersion     = domain.PageVersion
	TodoCount       = domain.TodoCount
	NoteCount       = domain.NoteCount
	DailyStat       = domain.DailyStat
	HeatmapDay      = domain.HeatmapDay
	SearchHit       = domain.SearchHit
	RepoMeta        = domain.RepoMeta
)
