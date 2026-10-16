package service

import (
	"testing"

	"repotory/internal/db"
	"repotory/internal/domain"
)

func mustNote(t *testing.T, svc *Service, projectID int64, content string) {
	t.Helper()
	if _, err := svc.CreateNote(projectID, content); err != nil {
		t.Fatalf("create note under project %d: %v", projectID, err)
	}
}

// ListOrganizeGroups returns only landing projects (default + placeholders) with
// their note counts, never a scanner-backed project.
func TestListOrganizeGroups(t *testing.T) {
	svc, _ := setupService(t)
	defID, err := db.GetDefaultProjectID(svc.db)
	if err != nil || defID <= 0 {
		t.Fatalf("default project must exist: id=%d err=%v", defID, err)
	}
	ph, err := db.EnsureImportPlaceholder(svc.db, "/code/ghost", "")
	if err != nil {
		t.Fatal(err)
	}
	real := seedProject(t, svc.db, "real", "/code/real")
	mustNote(t, svc, ph, "orphan one")
	mustNote(t, svc, ph, "orphan two")

	byID := map[int64]domain.OrganizeGroup{}
	for _, g := range svc.ListOrganizeGroups() {
		byID[g.ProjectID] = g
	}
	if _, ok := byID[real]; ok {
		t.Error("a scanned project must never appear in the landing groups")
	}
	g, ok := byID[ph]
	if !ok {
		t.Fatalf("placeholder %d missing from landing groups", ph)
	}
	if g.Kind != domain.ProjectKindImportPlaceholder || g.NoteCount != 2 {
		t.Errorf("placeholder group = %+v, want kind=%s notes=2", g, domain.ProjectKindImportPlaceholder)
	}
	if _, ok := byID[defID]; !ok {
		t.Error("the default bucket must always be offered in the landing groups")
	}
}

// ReassignLandingNotes moves every note out of a landing project and guards the
// direction: landing source -> (scanned | default) target only.
func TestReassignLandingNotes(t *testing.T) {
	svc, _ := setupService(t)
	ph, err := db.EnsureImportPlaceholder(svc.db, "/code/src", "")
	if err != nil {
		t.Fatal(err)
	}
	ph2, err := db.EnsureImportPlaceholder(svc.db, "/code/other-placeholder", "")
	if err != nil {
		t.Fatal(err)
	}
	target := seedProject(t, svc.db, "target", "/code/target")
	defID, _ := db.GetDefaultProjectID(svc.db)
	mustNote(t, svc, ph, "a")
	mustNote(t, svc, ph, "b")

	// source == target
	if _, err := svc.ReassignLandingNotes(target, target); err == nil {
		t.Error("same source/target must be rejected")
	}
	// scanned source (not a landing project)
	if _, err := svc.ReassignLandingNotes(target, ph); err == nil {
		t.Error("a scanned source must be rejected")
	}
	// placeholder target (just relocates the mess)
	if _, err := svc.ReassignLandingNotes(ph, ph2); err == nil {
		t.Error("a placeholder target must be rejected")
	}

	// valid: placeholder -> scanned
	moved, err := svc.ReassignLandingNotes(ph, target)
	if err != nil || moved != 2 {
		t.Fatalf("reassign moved=%d err=%v, want 2", moved, err)
	}
	if n, _ := db.CountProjectNotes(svc.db, ph); n != 0 {
		t.Errorf("source still holds %d notes after reassign", n)
	}
	if n, _ := db.CountProjectNotes(svc.db, target); n != 2 {
		t.Errorf("target holds %d notes, want 2", n)
	}
	// valid: placeholder -> default (unfile deliberately into the catch-all)
	ph3, _ := db.EnsureImportPlaceholder(svc.db, "/code/src3", "")
	mustNote(t, svc, ph3, "c")
	if moved, err := svc.ReassignLandingNotes(ph3, defID); err != nil || moved != 1 {
		t.Fatalf("reassign into default moved=%d err=%v, want 1", moved, err)
	}
}

// DeleteLandingProject protects the default and scanned projects and only drops a
// placeholder either when empty or with an explicit deleteNotes confirmation.
func TestDeleteLandingProject(t *testing.T) {
	svc, _ := setupService(t)
	defID, _ := db.GetDefaultProjectID(svc.db)
	if err := svc.DeleteLandingProject(defID, true); err == nil {
		t.Error("the default project must never be deletable")
	}
	real := seedProject(t, svc.db, "real", "/code/real")
	if err := svc.DeleteLandingProject(real, false); err == nil {
		t.Error("a scanned project must not be deletable from the organize surface")
	}

	ph, err := db.EnsureImportPlaceholder(svc.db, "/code/x", "")
	if err != nil {
		t.Fatal(err)
	}
	mustNote(t, svc, ph, "keepme")
	if err := svc.DeleteLandingProject(ph, false); err == nil {
		t.Error("a non-empty placeholder without deleteNotes must be rejected")
	}
	// notes survive; project still there
	if _, err := db.GetProjectByID(svc.db, ph); err != nil {
		t.Fatalf("placeholder should still exist after a rejected delete: %v", err)
	}
	if n, _ := db.CountProjectNotes(svc.db, ph); n != 1 {
		t.Fatalf("notes should be intact after a rejected delete, got %d", n)
	}
	if err := svc.DeleteLandingProject(ph, true); err != nil {
		t.Fatalf("confirmed delete: %v", err)
	}
	if _, err := db.GetProjectByID(svc.db, ph); err == nil {
		t.Error("placeholder should be gone after a confirmed delete")
	}

	// An emptied placeholder can be dropped without the confirmation.
	ph2, _ := db.EnsureImportPlaceholder(svc.db, "/code/empty", "")
	if err := svc.DeleteLandingProject(ph2, false); err != nil {
		t.Fatalf("empty placeholder should delete without confirmation: %v", err)
	}
}
