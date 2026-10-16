package service

import (
	"errors"
	"fmt"
	"log"

	"repotory/internal/db"
	"repotory/internal/domain"
)

// ListOrganizeGroups surfaces the landing projects (import placeholders and the
// default "Uncategorized" bucket) that still hold unfiled notes, so the
// knowledge-base organize view can offer them for reassignment (ADR-0023).
func (s *Service) ListOrganizeGroups() []domain.OrganizeGroup {
	groups, err := db.ListLandingGroups(s.db)
	if err != nil {
		log.Printf("organize: list landing groups failed: %v", err)
		return []domain.OrganizeGroup{}
	}
	if groups == nil {
		return []domain.OrganizeGroup{}
	}
	return groups
}

// ReassignLandingNotes moves every note out of a landing project into a real one
// (or the default bucket) and returns the count moved. The source must be a
// landing project; the target must be a scanner-backed project or the default —
// never another placeholder (that would just relocate the mess), and never the
// source itself.
func (s *Service) ReassignLandingNotes(sourceID, targetID int64) (int, error) {
	if sourceID == targetID {
		return 0, errors.New("organize: source and target are the same project")
	}
	src, err := db.GetProjectByID(s.db, sourceID)
	if err != nil {
		return 0, fmt.Errorf("organize: source project not found: %w", err)
	}
	if src.ProjectKind != domain.ProjectKindImportPlaceholder && src.ProjectKind != domain.ProjectKindDefault {
		return 0, fmt.Errorf("organize: source %d (kind %q) is not a landing project", sourceID, src.ProjectKind)
	}
	tgt, err := db.GetProjectByID(s.db, targetID)
	if err != nil {
		return 0, fmt.Errorf("organize: target project not found: %w", err)
	}
	if tgt.ProjectKind != domain.ProjectKindScanned && tgt.ProjectKind != domain.ProjectKindDefault {
		return 0, fmt.Errorf("organize: target %d (kind %q) is not a real project or the default bucket", targetID, tgt.ProjectKind)
	}
	moved, err := db.ReassignLandingNotes(s.db, sourceID, targetID)
	if err != nil {
		return 0, fmt.Errorf("organize: reassign notes: %w", err)
	}
	log.Printf("organize: moved %d note(s) from project %d to %d", moved, sourceID, targetID)
	return moved, nil
}

// DeleteLandingProject removes an import-placeholder project from the organize
// surface. The default project and scanned projects can never be deleted here
// (the default is the protected fallback; scanned projects are out of scope).
// A placeholder that still holds notes is only removable when deleteNotes is
// true — this is the user-confirmed "source repo gone → also delete its notes"
// path, and it is deliberately explicit rather than an automatic cascade.
func (s *Service) DeleteLandingProject(id int64, deleteNotes bool) error {
	p, err := db.GetProjectByID(s.db, id)
	if err != nil {
		return fmt.Errorf("organize: project not found: %w", err)
	}
	switch p.ProjectKind {
	case domain.ProjectKindScanned:
		return fmt.Errorf("organize: refusing to delete scanned project %d from the organize surface", id)
	case domain.ProjectKindDefault:
		return errors.New("organize: the default project cannot be deleted")
	}
	if !deleteNotes {
		if n, _ := db.CountProjectNotes(s.db, id); n > 0 {
			return fmt.Errorf("organize: project %d still holds %d note(s); reassign them first or confirm deleting them", id, n)
		}
	}
	if _, err := db.DeleteLandingProject(s.db, id); err != nil {
		return fmt.Errorf("organize: delete placeholder: %w", err)
	}
	return nil
}
