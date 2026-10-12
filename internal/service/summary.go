package service

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"repotory/internal/db"
	"repotory/internal/domain"
	"repotory/internal/integrity"
)

// SummaryData holds the daily summary payload.
type SummaryData struct {
	Date         string `json:"date"`
	RepoCount    int    `json:"repo_count"`
	TotalFiles   int    `json:"total_files"`
	TotalAdded   int    `json:"total_added"`
	TotalDeleted int    `json:"total_deleted"`
	MyAdded      int    `json:"my_added"`
	MyDeleted    int    `json:"my_deleted"`
	MyFiles      int    `json:"my_files"`
	IsWorkday    bool   `json:"is_workday"`
}

// GetSummary returns aggregated stats for all repositories on a given date.
func (s *Service) GetSummary(date string) (*SummaryData, error) {
	if date == "" {
		date = s.git.GetYesterdayDate()
	}
	if err := s.git.ValidateDate(date); err != nil {
		return nil, fmt.Errorf("invalid date format: %w", err)
	}

	allStats, err := db.GetStatsByDate(s.db, date)
	if err != nil {
		return nil, fmt.Errorf("failed to load summary: %w", err)
	}

	summary := &SummaryData{Date: date, IsWorkday: s.git.IsWorkday(date)}
	repoSet := make(map[int64]bool)
	for _, st := range allStats {
		repoSet[st.RepositoryID] = true
		summary.TotalFiles += st.FilesChanged
		summary.TotalAdded += st.LinesAdded
		summary.TotalDeleted += st.LinesDeleted
		if st.Author == s.gitUser() {
			summary.MyAdded += st.LinesAdded
			summary.MyDeleted += st.LinesDeleted
			summary.MyFiles += st.FilesChanged
		}
	}
	summary.RepoCount = len(repoSet)
	return summary, nil
}

// HeatmapResponse holds heatmap data for the frontend.
type HeatmapResponse struct {
	Days []domain.HeatmapDay `json:"days"`
}

// GetHeatmapData returns daily commit stats for the past year, optionally
// restricted to a single project (projectID > 0, used by the project detail
// page); projectID <= 0 aggregates across all repositories.
func (s *Service) GetHeatmapData(projectID int64) *HeatmapResponse {
	endDate := s.git.GetTodayDate()
	startDate := time.Now().AddDate(0, 0, -statsBackfillDays).Format("2006-01-02")

	days, err := db.GetHeatmapData(s.db, startDate, endDate, s.gitUser(), projectID)
	if err != nil {
		log.Printf("get heatmap error: %v", err)
		return &HeatmapResponse{Days: []domain.HeatmapDay{}}
	}
	if days == nil {
		days = []domain.HeatmapDay{}
	}
	return &HeatmapResponse{Days: days}
}

// StatusBarData holds real-time status information.
type StatusBarData struct {
	CurrentTime      string `json:"current_time"`
	LastCommitTime   string `json:"last_commit_time"`
	LastCommitRepo   string `json:"last_commit_repo"`
	LastCommitBranch string `json:"last_commit_branch"`
	LastCommitMsg    string `json:"last_commit_msg"`
}

// statusCacheTTL is how long the status bar cache lasts before refreshing.
const statusCacheTTL = 30 * time.Second

// GetStatusBar returns current status bar information with 30-second caching
// to avoid running git log on every UI render. Uses double-check locking to
// avoid holding the mutex during the (potentially slow) git command.
func (s *Service) GetStatusBar() *StatusBarData {
	s.statusCacheMu.Lock()
	if s.statusCache != nil && time.Now().Sub(s.statusCacheTime) < statusCacheTTL {
		cached := s.statusCache
		s.statusCacheMu.Unlock()
		return cached
	}
	s.statusCacheMu.Unlock()

	repos, _ := db.GetAllRepositories(s.db)
	repoPaths := make([]string, 0, len(repos))
	for _, r := range repos {
		repoPaths = append(repoPaths, r.Path)
	}

	now := time.Now()
	data := &StatusBarData{
		CurrentTime: now.Format("2006-01-02 15:04:05"),
	}

	recent, err := s.git.GetRecentCommit(repoPaths, s.gitUser())
	if err == nil && recent != nil {
		data.LastCommitTime = recent.Time
		data.LastCommitRepo = filepath.Base(recent.Repo)
		data.LastCommitBranch = recent.Branch
		data.LastCommitMsg = recent.Message
	}

	s.statusCacheMu.Lock()
	s.statusCache = data
	s.statusCacheTime = now
	s.statusCacheMu.Unlock()
	return data
}

// GetTodoCounts returns incomplete and total todo counts per project.
func (s *Service) GetTodoCounts() []domain.TodoCount {
	counts, err := db.GetTodoCounts(s.db)
	if err != nil {
		log.Printf("get todo counts error: %v", err)
		return nil
	}
	if counts == nil {
		counts = []domain.TodoCount{}
	}
	return counts
}

// GetNoteCounts returns the count of notes per project.
func (s *Service) GetNoteCounts() []domain.NoteCount {
	counts, err := db.GetNoteCounts(s.db)
	if err != nil {
		log.Printf("get note counts error: %v", err)
		return nil
	}
	if counts == nil {
		counts = []domain.NoteCount{}
	}
	return counts
}

// RunIntegrityChecks audits how much of the knowledge base can be trusted:
// FTS index drift, orphan rows, schema shape versus the version stamp, scan
// coverage and mined-cache freshness.
//
// It is exposed on the Service rather than by handing out the *sql.DB so that
// the integrity package stays behind this layer, per ADR-0005. The returned
// Report is a snapshot - it is read-only and safe to keep or print.
func (s *Service) RunIntegrityChecks() *integrity.Report {
	return integrity.RunAll(s.db)
}
