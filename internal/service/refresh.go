package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"repo-nest/internal/db"
	"repo-nest/internal/domain"
)

// statsBackfillDays is how far back the range refreshers query git history.
const statsBackfillDays = 365

// refreshRepoStatsRange queries per-day git stats for one repository between
// startDate and endDate (inclusive) and upserts the non-zero rows. It stores
// the aggregate "all" author row and, when a personal git user is configured,
// a separate row for that author. This is the single loop shared by every
// refresh path (scan, project history backfill); it respects ctx cancellation.
func (s *Service) refreshRepoStatsRange(ctx context.Context, repo domain.Repository, startDate, endDate string, logErrors bool) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	authors := []string{""} // "" queries the aggregate across all authors
	if s.gitUser() != "" {
		authors = append(authors, s.gitUser())
	}
	for _, author := range authors {
		entries, err := s.git.QueryStatsRange(repo.Path, startDate, endDate, author)
		if err != nil {
			if logErrors {
				log.Printf("refresh stats query error (repo %s, author %q): %v", repo.Path, author, err)
			}
			continue
		}
		for _, e := range entries {
			if e.FilesChanged > 0 || e.LinesAdded > 0 || e.LinesDeleted > 0 {
				storeAuthor := author
				if storeAuthor == "" {
					storeAuthor = "all"
				}
				if err := db.UpsertDailyStat(s.db, repo.ID, e.Date, storeAuthor,
					e.FilesChanged, e.LinesAdded, e.LinesDeleted, e.Commits); err != nil && logErrors {
					log.Printf("refresh stats upsert error (repo %s, %s): %v", repo.Path, e.Date, err)
				}
			}
		}
	}
}

// refreshCollectedStats refreshes the full history window for the repos of
// the given (collected) projects. Used at the end of a scan. The scan status
// reports backfilling while this runs so the UI can distinguish history
// backfill from repo discovery.
func (s *Service) refreshCollectedStats(ctx context.Context, collectedIDs []int64) {
	s.scanMu.Lock()
	s.scanBackfilling = true
	s.scanMu.Unlock()
	defer func() {
		s.scanMu.Lock()
		s.scanBackfilling = false
		s.scanMu.Unlock()
	}()

	startDate := time.Now().AddDate(0, 0, -statsBackfillDays).Format("2006-01-02")
	endDate := s.git.GetTodayDate()

	for _, projectID := range collectedIDs {
		select {
		case <-ctx.Done():
			log.Printf("stats refresh cancelled")
			return
		default:
		}
		repos, err := db.GetRepositoriesByProjectID(s.db, projectID)
		if err != nil {
			continue
		}
		for _, repo := range repos {
			s.refreshRepoStatsRange(ctx, repo, startDate, endDate, false)
		}
	}
}

// RefreshProjectHistory backfills a full year of daily stats for all repos
// belonging to a single project. Only called on explicit user action (button
// click) so we don't scan history for uncollected repos.
func (s *Service) RefreshProjectHistory(ctx context.Context, projectID int64) error {
	repos, err := db.GetRepositoriesByProjectID(s.db, projectID)
	if err != nil {
		return fmt.Errorf("failed to load repos: %w", err)
	}
	if len(repos) == 0 {
		return fmt.Errorf("project has no repositories")
	}

	startDate := time.Now().AddDate(0, 0, -statsBackfillDays).Format("2006-01-02")
	endDate := s.git.GetTodayDate()

	for _, repo := range repos {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		s.refreshRepoStatsRange(ctx, repo, startDate, endDate, true)
		if err := db.UpdateRepositoryLastScanned(s.db, repo.ID); err != nil {
			log.Printf("refresh history update last_scanned error (repo %d): %v", repo.ID, err)
		}
	}
	return nil
}

// refreshProjectStatsForDate performs the on-demand single-day refresh used
// when the dashboard requests a project's stats for today/yesterday and the
// database has no row yet. Deduplicated by (project, date): the dashboard
// polls, so without the guard every poll for a date with no rows yet would
// spawn another full round of git subprocesses per repository.
func (s *Service) refreshProjectStatsForDate(projectID int64, date string) {
	key := strconv.FormatInt(projectID, 10) + "|" + date
	if _, loaded := s.statsRefreshInFlight.LoadOrStore(key, true); loaded {
		return
	}
	defer s.statsRefreshInFlight.Delete(key)

	repos, err := db.GetRepositoriesByProjectID(s.db, projectID)
	if err != nil {
		return
	}
	// Query both the aggregate ("all") and the personal author, matching the
	// behaviour of refreshRepoStatsRange so the dashboard has consistent data.
	authors := []string{""}
	if s.gitUser() != "" {
		authors = append(authors, s.gitUser())
	}
	for _, r := range repos {
		for _, author := range authors {
			result, qErr := s.git.QueryStats(r.Path, date, author)
			if qErr != nil || result == nil {
				continue
			}
			storeAuthor := author
			if storeAuthor == "" {
				storeAuthor = "all"
			}
			// Mirror the non-zero guard in refreshRepoStatsRange: when QueryStats
			// fails it returns an all-zero Result, and writing that row would make
			// the dashboard show "0" instead of "no data", masking the error.
			if result.FilesChanged > 0 || result.LinesAdded > 0 || result.LinesDeleted > 0 {
				_ = db.UpsertDailyStat(s.db, r.ID, date, storeAuthor,
					result.FilesChanged, result.LinesAdded, result.LinesDeleted, result.Commits)
			}
		}
	}
}
