package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"repo-nest/internal/db"
	"repo-nest/internal/domain"
	"repo-nest/internal/knowledge"
	"repo-nest/internal/stats"
)

// ErrProjectNotFound marks the "no such project" case distinctly from a
// storage failure: GetProjectByID's error used to collapse into a plain
// "project not found", so a full disk or a locked database surfaced as 404
// and neither an operator nor an agent could tell "absent" from "broken".
// Transports (httpapi maps it to 404, everything else to 500) should check
// it with errors.Is.
var ErrProjectNotFound = errors.New("project not found")

// GetProjectDetail returns a project with all its repositories and stats.
func (s *Service) GetProjectDetail(id int64) (*ProjectDetailResponse, error) {
	project, err := db.GetProjectByID(s.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("load project %d: %w", id, err)
	}
	repos, _ := db.GetRepositoriesByProjectID(s.db, id)

	repoList := make([]RepoWithStats, 0, len(repos))
	for _, repo := range repos {
		statsList, _ := db.GetStatsByRepositoryAndDate(s.db, repo.ID, "")
		if statsList == nil {
			statsList = []domain.DailyStat{}
		}
		repoList = append(repoList, RepoWithStats{Repository: repo, Stats: statsList})
	}
	return &ProjectDetailResponse{Project: project, Repos: repoList}, nil
}

// GetProjectStats returns daily stats for a project, optionally filtered by
// date, with the same on-demand refresh as GetProjects.
func (s *Service) GetProjectStats(id int64, date string) []domain.DailyStat {
	if date == "" {
		date = s.git.GetYesterdayDate()
	}
	if err := s.git.ValidateDate(date); err != nil {
		return nil
	}
	statsList, err := db.GetStatsByProject(s.db, id, date)
	if err != nil {
		log.Printf("get stats error: %v", err)
		return nil
	}
	// On-demand refresh runs in the background so this call never blocks on git.
	// The next load returns the back-filled rows. refreshProjectStatsForDate
	// deduplicates by (project, date), so a dashboard polling this endpoint
	// cannot stack up one git process storm per load.
	if len(statsList) == 0 {
		s.bgGo(fmt.Sprintf("stats refresh project %d %s", id, date), func(_ context.Context) {
			s.refreshProjectStatsForDate(id, date)
		})
	}
	return statsList
}

// UpdateProjectLevel adjusts a project's grouping: "down" splits a multi-repo
// project into per-repo projects, "up" merges sibling projects sharing the
// same parent directory into this one. Both run as a single transaction in
// the db layer.
func (s *Service) UpdateProjectLevel(id int64, direction string) (*LevelUpdateResult, error) {
	var newLevel int
	var err error
	switch direction {
	case "down":
		newLevel, err = db.SplitProjectDown(s.db, id)
	case "up":
		newLevel, err = db.MergeProjectUp(s.db, id)
	default:
		return nil, fmt.Errorf("direction must be 'up' or 'down'")
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	return &LevelUpdateResult{Success: true, NewLevel: newLevel}, nil
}

// ToggleStar flips the starred status of a project.
func (s *Service) ToggleStar(projectID int64) (bool, error) {
	return db.ToggleProjectStar(s.db, projectID)
}

// SearchProjects searches for projects by name or path and enriches the
// results with yesterday's stats.
func (s *Service) SearchProjects(query string) []ProjectResponse {
	projects, err := db.SearchProjects(s.db, query)
	if err != nil {
		log.Printf("search projects error: %v", err)
		return nil
	}
	return s.enrichProjects(projects, s.git.GetYesterdayDate())
}

// ProjectOverview is the mined-knowledge payload for a project detail page.
type ProjectOverview struct {
	ReadmeExcerpt   string                     `json:"readme_excerpt"`
	TechStack       []knowledge.Tech           `json:"tech_stack"`
	Languages       []knowledge.LanguageStat   `json:"languages"`
	Dependencies    []knowledge.Dependency     `json:"dependencies"`
	TopContributors []knowledge.TopContributor `json:"top_contributors"`
	Activity        *knowledge.ActivityStat    `json:"activity"`
	RecentCommits   []stats.RecentCommit       `json:"recent_commits"`
	Cached          bool                       `json:"cached"`
	Mining          bool                       `json:"mining,omitempty"`
}

// GetProjectOverview returns mined knowledge for a project: README excerpt,
// detected tech stack, language breakdown and recent commits. Mined results
// are cached in repo_meta so repeated loads do not re-walk the working tree.
func (s *Service) GetProjectOverview(projectID int64) (*ProjectOverview, error) {
	project, err := db.GetProjectByID(s.db, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("load project %d: %w", projectID, err)
	}
	repos, _ := db.GetRepositoriesByProjectID(s.db, projectID)

	resp := &ProjectOverview{}

	// Cache is keyed by the first repo id when available.
	var cacheRepoID int64
	if len(repos) > 0 {
		cacheRepoID = repos[0].ID
	}
	if cacheRepoID > 0 {
		if meta, err := db.GetRepoMeta(s.db, cacheRepoID); err == nil && meta != nil && meta.TechStack != "" {
			_ = json.Unmarshal([]byte(meta.TechStack), &resp.TechStack)
			_ = json.Unmarshal([]byte(meta.Languages), &resp.Languages)
			_ = json.Unmarshal([]byte(meta.Dependencies), &resp.Dependencies)
			_ = json.Unmarshal([]byte(meta.TopContributors), &resp.TopContributors)
			_ = json.Unmarshal([]byte(meta.Activity), &resp.Activity)
			resp.ReadmeExcerpt = meta.ReadmeExcerpt
			resp.Cached = true
		}
	}

	// Mine fresh when no cache was found — trigger async so the API returns quickly.
	if !resp.Cached {
		resp.Mining = true
		s.bgGo(fmt.Sprintf("mine project %d", projectID), func(_ context.Context) {
			s.mineAndCache(cacheRepoID, project.RootPath, repos)
		})
	}
	// Recent commits are always fresh.
	repoPaths := make([]string, 0, len(repos))
	for _, r := range repos {
		repoPaths = append(repoPaths, r.Path)
	}
	if commits, err := s.git.GetRecentCommits(repoPaths, s.gitUser(), 8); err == nil && commits != nil {
		resp.RecentCommits = commits
	}
	// Normalize nil slices to empty (non-nil) so JSON consumers never see
	// `null` for a field documented as an array. A nil slice marshals to JSON
	// `null`, which crashes frontends that read `field.length` directly
	// (e.g. ProjectOverviewSection on the project detail page).
	if resp.TechStack == nil {
		resp.TechStack = []knowledge.Tech{}
	}
	if resp.Languages == nil {
		resp.Languages = []knowledge.LanguageStat{}
	}
	if resp.Dependencies == nil {
		resp.Dependencies = []knowledge.Dependency{}
	}
	if resp.TopContributors == nil {
		resp.TopContributors = []knowledge.TopContributor{}
	}
	if resp.RecentCommits == nil {
		resp.RecentCommits = []stats.RecentCommit{}
	}
	return resp, nil
}

// mineAndCache runs knowledge mining in the background and caches the result.
// It uses a sync.Map to skip duplicate mining requests for the same repo.
func (s *Service) mineAndCache(cacheRepoID int64, rootPath string, repos []domain.Repository) {
	// Dedup: if a mining goroutine is already running for this repo, skip.
	if cacheRepoID > 0 {
		if _, loaded := s.miningInFlight.LoadOrStore(cacheRepoID, true); loaded {
			return
		}
		defer s.miningInFlight.Delete(cacheRepoID)
	}
	minePath := rootPath
	if minePath == "" && len(repos) > 0 {
		minePath = repos[0].Path
	}
	if minePath == "" {
		return
	}
	k, err := knowledge.Mine(minePath)
	if err != nil || k == nil {
		return
	}
	// For a multi-repo grouping the container directory is not a git
	// repository, so Mine's per-path git probes (activity, top contributors)
	// silently returned zero. Aggregate across the actual repositories
	// instead — languages/tech stack walk the working tree and are already
	// container-safe.
	if len(repos) > 1 {
		paths := make([]string, 0, len(repos))
		for _, r := range repos {
			paths = append(paths, r.Path)
		}
		k.Activity = knowledge.AggregateActivity(paths)
		k.TopContributors = knowledge.AggregateContributors(paths, 5)
	}
	if cacheRepoID > 0 {
		ts, _ := json.Marshal(k.TechStack)
		ls, _ := json.Marshal(k.Languages)
		ds, _ := json.Marshal(k.Dependencies)
		tc, _ := json.Marshal(k.TopContributors)
		aa, _ := json.Marshal(k.Activity)
		if err := db.UpsertRepoMeta(s.db, cacheRepoID, string(ts), k.ReadmeExcerpt, string(ls), string(ds), string(tc), string(aa)); err != nil {
			log.Printf("mineAndCache: upsert repo_meta for repo %d failed: %v", cacheRepoID, err)
		}
	}
}

// ProjectSummary is the compact project payload used by the CLI and the MCP
// server: the project record with its repositories.
type ProjectSummary struct {
	Project   *domain.Project     `json:"project"`
	Repos     []domain.Repository `json:"repos"`
	RepoCount int                 `json:"repo_count"`
}

// GetProjectSummary returns a project with its repositories for read-only
// consumers (CLI / MCP).
func (s *Service) GetProjectSummary(id int64) (*ProjectSummary, error) {
	project, err := db.GetProjectByID(s.db, id)
	if err != nil {
		return nil, err
	}
	repos, err := db.GetRepositoriesByProjectID(s.db, id)
	if err != nil {
		return nil, err
	}
	return &ProjectSummary{Project: project, Repos: repos, RepoCount: len(repos)}, nil
}

// ListProjects returns all projects for read-only consumers (CLI / MCP).
func (s *Service) ListProjects() []domain.Project {
	projects, err := db.GetAllProjects(s.db)
	if err != nil {
		log.Printf("list projects error: %v", err)
		return nil
	}
	return projects
}
