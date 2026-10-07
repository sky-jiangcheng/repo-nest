package service

import (
	"context"
	"log"
	"strconv"

	"repo-nest/internal/db"
	"repo-nest/internal/domain"
)

// defaultDailyCodeStandard is the fallback daily line standard used when the
// daily_code_standard config key is unset or unparsable.
const defaultDailyCodeStandard = 500

// ProjectResponse is the enriched project payload sent to the frontend.
type ProjectResponse struct {
	domain.Project
	RepoCount     int  `json:"repo_count"`
	TotalAdded    int  `json:"total_added"`
	TotalDeleted  int  `json:"total_deleted"`
	MyAdded       int  `json:"my_added"`
	MyDeleted     int  `json:"my_deleted"`
	MyFiles       int  `json:"my_files"`
	IsWorkday     bool `json:"is_workday"`
	BelowStandard bool `json:"below_standard"`
}

// RepoWithStats is a repository record with embedded daily stats.
type RepoWithStats struct {
	domain.Repository
	Stats []domain.DailyStat `json:"stats"`
	// WebURL is the browsable commit log for the repo's origin remote, or ""
	// when it has none the UI can link to. Lets a repo row offer the real
	// history instead of per-author line counts, which are aggregates the
	// reader cannot act on.
	WebURL string `json:"web_url"`
}

// ProjectDetailResponse is the full project detail payload.
type ProjectDetailResponse struct {
	*domain.Project
	Repos []RepoWithStats `json:"repos"`
}

// LevelUpdateResult holds the result of a level change operation.
type LevelUpdateResult struct {
	Success  bool `json:"success"`
	NewLevel int  `json:"new_level"`
}

// dailyCodeStandard reads the configured daily line standard.
func (s *Service) dailyCodeStandard() int {
	v, _ := db.GetConfig(s.db, "daily_code_standard")
	n, _ := strconv.Atoi(v)
	if n == 0 {
		return defaultDailyCodeStandard
	}
	return n
}

// enrichProjects builds ProjectResponse entries (repo counts, personal vs
// total splits, workday standard check) for the given projects on a date.
func (s *Service) enrichProjects(projects []domain.Project, date string) []ProjectResponse {
	codeStd := s.dailyCodeStandard()
	isWorkday := s.git.IsWorkday(date)

	result := make([]ProjectResponse, 0, len(projects))
	for _, p := range projects {
		statsList, _ := db.GetStatsByProject(s.db, p.ID, date)
		repos, _ := db.GetRepositoriesByProjectID(s.db, p.ID)

		pr := ProjectResponse{
			Project:   p,
			RepoCount: len(repos),
			IsWorkday: isWorkday,
		}
		for _, st := range statsList {
			pr.TotalAdded += st.LinesAdded
			pr.TotalDeleted += st.LinesDeleted
			if st.Author == s.gitUser() {
				pr.MyAdded += st.LinesAdded
				pr.MyDeleted += st.LinesDeleted
				pr.MyFiles += st.FilesChanged
			}
		}
		pr.BelowStandard = isWorkday && pr.MyAdded < codeStd
		result = append(result, pr)
	}
	return result
}

// GetProjects returns enriched project summaries, optionally filtered by date
// and starred status. Triggers an on-demand single-day stats refresh for
// today/yesterday when a project has no rows yet.
func (s *Service) GetProjects(date string, starredOnly bool) []ProjectResponse {
	if date == "" {
		date = s.git.GetYesterdayDate()
	}
	if err := s.git.ValidateDate(date); err != nil {
		log.Printf("invalid date: %v", err)
		return nil
	}

	var projects []domain.Project
	var err error
	if starredOnly {
		projects, err = db.GetStarredProjects(s.db)
	} else {
		projects, err = db.GetAllProjects(s.db)
	}
	if err != nil {
		log.Printf("get projects error: %v", err)
		return nil
	}

	// Only auto-refresh for today or yesterday to avoid triggering git scans
	// on historical dates. Run it off the request path so the first open of
	// the dashboard/overview never blocks on git subprocesses; the next load
	// sees the back-filled rows. bgGo (not a bare `go`): a panic in a bare
	// background goroutine kills the whole process, and Shutdown must wait
	// for it before the database handle closes.
	today := s.git.GetTodayDate()
	yesterday := s.git.GetYesterdayDate()
	if date == today || date == yesterday {
		s.bgGo("refresh missing stats", func(_ context.Context) {
			s.refreshMissingStatsForDate(projects, date)
		})
	}

	return s.enrichProjects(projects, date)
}

// refreshMissingStatsForDate back-fills per-day stats for projects that have no
// row yet for date. Meant to run in its own goroutine (see GetProjects).
func (s *Service) refreshMissingStatsForDate(projects []domain.Project, date string) {
	for _, p := range projects {
		statsList, _ := db.GetStatsByProject(s.db, p.ID, date)
		if len(statsList) == 0 {
			if repos, _ := db.GetRepositoriesByProjectID(s.db, p.ID); len(repos) > 0 {
				s.refreshProjectStatsForDate(p.ID, date)
			}
		}
	}
}
