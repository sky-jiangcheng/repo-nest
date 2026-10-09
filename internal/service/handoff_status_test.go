package service

import (
	"testing"
)

func TestLatestHandoff(t *testing.T) {
	svc, _ := setupService(t)
	pid1 := seedProject(t, svc.db, "One", "/tmp/one")
	pid2 := seedProject(t, svc.db, "Two", "/tmp/two")

	if status, err := svc.LatestHandoff(0); err != nil || status != nil {
		t.Fatalf("empty database: status=%v err=%v, want nil,nil", status, err)
	}

	if _, err := svc.CreateHandoffNote(HandoffInput{ProjectID: pid1, Summary: "first", Changes: []string{"one"}}); err != nil {
		t.Fatalf("create first handoff: %v", err)
	}
	if _, err := svc.CreateNoteWithMeta(pid2, "older handoff shape", "not one", "handoff-ish", "knowledge", "test"); err != nil {
		t.Fatalf("create decoy: %v", err)
	}
	second, err := svc.CreateHandoffNote(HandoffInput{ProjectID: pid2, Summary: "second", Changes: []string{"two"}})
	if err != nil {
		t.Fatalf("create second handoff: %v", err)
	}

	global, err := svc.LatestHandoff(0)
	if err != nil {
		t.Fatalf("global latest: %v", err)
	}
	if global == nil || global.NoteID != second.NoteID || global.ProjectID != pid2 {
		t.Fatalf("global latest = %+v, want note %d in project %d", global, second.NoteID, pid2)
	}
	if global.UpdatedAt == "" || global.Title == "" {
		t.Errorf("status surface incomplete: %+v", global)
	}

	scoped, err := svc.LatestHandoff(pid1)
	if err != nil {
		t.Fatalf("project latest: %v", err)
	}
	if scoped == nil || scoped.ProjectID != pid1 {
		t.Fatalf("project latest = %+v, want project %d", scoped, pid1)
	}
}
