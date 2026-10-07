package db

import "database/sql"

// UpsertDailyStat inserts or updates a daily stat row by (repo, date, author).
func UpsertDailyStat(db *sql.DB, repoID int64, date, author string, files, added, deleted, commits int) error {
	_, err := db.Exec(
		"INSERT INTO daily_stats (repository_id, stat_date, author, files_changed, lines_added, lines_deleted, commits) VALUES (?, ?, ?, ?, ?, ?, ?) "+
			"ON CONFLICT(repository_id, stat_date, author) DO UPDATE SET files_changed = excluded.files_changed, lines_added = excluded.lines_added, lines_deleted = excluded.lines_deleted, commits = excluded.commits",
		repoID, date, author, files, added, deleted, commits)
	return err
}

// GetStatsByProject returns daily stats for a project, optionally for one date.
func GetStatsByProject(db *sql.DB, projectID int64, date string) ([]DailyStat, error) {
	q := "SELECT d.id, d.repository_id, d.stat_date, d.author, d.files_changed, d.lines_added, d.lines_deleted " +
		"FROM daily_stats d JOIN repositories r ON r.id = d.repository_id WHERE r.project_id = ?"
	if date != "" {
		q += " AND d.stat_date = ?"
	}
	q += " ORDER BY d.stat_date DESC, d.author ASC"
	var rows *sql.Rows
	var err error
	if date != "" {
		rows, err = db.Query(q, projectID, date)
	} else {
		rows, err = db.Query(q, projectID)
	}
	return scanDailyStats(rows, err)
}

// GetStatsByRepositoryAndDate returns daily stats for a repo, optionally one date.
func GetStatsByRepositoryAndDate(db *sql.DB, repoID int64, date string) ([]DailyStat, error) {
	q := "SELECT id, repository_id, stat_date, author, files_changed, lines_added, lines_deleted FROM daily_stats WHERE repository_id = ?"
	if date != "" {
		q += " AND stat_date = ?"
	}
	q += " ORDER BY stat_date DESC, author ASC"
	var rows *sql.Rows
	var err error
	if date != "" {
		rows, err = db.Query(q, repoID, date)
	} else {
		rows, err = db.Query(q, repoID)
	}
	return scanDailyStats(rows, err)
}

// GetStatsByDate returns all daily stats for a given date.
func GetStatsByDate(db *sql.DB, date string) ([]DailyStat, error) {
	rows, err := db.Query("SELECT id, repository_id, stat_date, author, files_changed, lines_added, lines_deleted FROM daily_stats WHERE stat_date = ?", date)
	return scanDailyStats(rows, err)
}

// GetHeatmapData returns per-day aggregated stats between start and end.
// A non-empty gitUser restricts the aggregation to that author. A positive
// projectID restricts it to the repositories of that project (used by the
// project detail page); projectID <= 0 aggregates across all repositories.
func GetHeatmapData(db *sql.DB, start, end, gitUser string, projectID int64) ([]HeatmapDay, error) {
	q := "SELECT d.stat_date, COALESCE(SUM(d.lines_added),0), COALESCE(SUM(d.lines_deleted),0), COALESCE(SUM(d.commits),0) " +
		"FROM daily_stats d "
	var args []any
	if projectID > 0 {
		q += "JOIN repositories r ON r.id = d.repository_id WHERE r.project_id = ? "
		args = append(args, projectID)
		q += "AND d.stat_date BETWEEN ? AND ? "
	} else {
		q += "WHERE d.stat_date BETWEEN ? AND ? "
	}
	args = append(args, start, end)
	if gitUser != "" {
		q += "AND d.author = ? "
		args = append(args, gitUser)
	}
	q += "GROUP BY d.stat_date ORDER BY d.stat_date ASC"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var days []HeatmapDay
	for rows.Next() {
		var d HeatmapDay
		// stat_date may carry a time component depending on how the row was
		// written; the heatmap contract is a plain YYYY-MM-DD calendar day.
		// Scanning verbatim leaked "2025-10-11T00:00:00Z" to the frontend,
		// whose grid keys on local YYYY-MM-DD — every cell missed and the
		// heatmap rendered blank while the header totals (summed straight
		// from these rows) stayed correct.
		var date string
		if err := rows.Scan(&date, &d.LinesAdded, &d.LinesDeleted, &d.Commits); err != nil {
			return nil, err
		}
		if len(date) > 10 {
			date = date[:10]
		}
		d.Date = date
		days = append(days, d)
	}
	return days, rows.Err()
}

func scanDailyStats(rows *sql.Rows, err error) ([]DailyStat, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []DailyStat
	for rows.Next() {
		var s DailyStat
		if err := rows.Scan(&s.ID, &s.RepositoryID, &s.StatDate, &s.Author, &s.FilesChanged, &s.LinesAdded, &s.LinesDeleted); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}
