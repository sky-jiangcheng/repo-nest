package service

import (
	"repo-nest/internal/db"
	"repo-nest/internal/domain"
)

// ListPageVersions returns the recent version history for a wiki page
// (ADR-0021). The page-side twin of ListNoteVersions: at most 50 snapshots,
// newest first.
func (s *Service) ListPageVersions(pageID int64) []domain.PageVersion {
	versions, err := db.ListPageVersions(s.db, pageID)
	if err != nil {
		return []domain.PageVersion{}
	}
	return versions
}

// RestorePageVersion rolls a page back to a previous version (ADR-0021 决策 3):
// the current content is preserved as its own version first, then the old
// content is applied through the same UpdateWikiPage write path. History only
// grows. Replaying the same version is a no-op.
func (s *Service) RestorePageVersion(pageID, versionID int64) error {
	return db.RestorePageVersion(s.db, pageID, versionID)
}

// DiffPageVersions returns a line-based diff between a historical version and
// the current page content (same diff.Lines algorithm as the note side).
func (s *Service) DiffPageVersions(pageID, versionID int64) (string, error) {
	return db.DiffPageVersions(s.db, pageID, versionID)
}
