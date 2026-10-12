package service

import (
	"fmt"
	"strings"
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

// TestListRecentHandoffs pins the memory-ring service surface (D25): decoys
// excluded, newest first, project name joined, the auto-captured badge, and
// the content-free guarantee — the ring must never ship note bodies.
func TestListRecentHandoffs(t *testing.T) {
	svc, _ := setupService(t)
	pid1 := seedProject(t, svc.db, "One", "/tmp/one")
	pid2 := seedProject(t, svc.db, "Two", "/tmp/two")

	if got := svc.ListRecentHandoffs(5); len(got) != 0 {
		t.Fatalf("empty database: got %+v, want empty", got)
	}

	h1, err := svc.CreateHandoffNote(HandoffInput{ProjectID: pid1, Summary: "first", Changes: []string{"one"}})
	if err != nil {
		t.Fatalf("create first handoff: %v", err)
	}
	h2, err := svc.CreateHandoffNote(HandoffInput{ProjectID: pid2, Summary: "second", Changes: []string{"two"}})
	if err != nil {
		t.Fatalf("create second handoff: %v", err)
	}
	// Decoys: a plain note and a handoff-ish tag must not enter the ring.
	if _, err := svc.CreateNoteWithMeta(pid1, "plain note", "plain body", "", "knowledge", "test"); err != nil {
		t.Fatalf("seed plain: %v", err)
	}
	if _, err := svc.CreateNoteWithMeta(pid1, "handoff-ish decoy", "decoy body", "handoff-ish", "knowledge", "test"); err != nil {
		t.Fatalf("seed decoy: %v", err)
	}
	auto, err := svc.CreateNoteWithMeta(pid1, "Session Handoff auto", "auto body", "handoff, auto-captured", "knowledge", "mcp")
	if err != nil {
		t.Fatalf("seed auto handoff: %v", err)
	}

	got := svc.ListRecentHandoffs(10)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(got), got)
	}
	byID := map[int64]HandoffEntry{}
	for _, e := range got {
		byID[e.NoteID] = e
		if e.ProjectName == "" {
			t.Errorf("project name missing: %+v", e)
		}
		if e.Title == "" || e.UpdatedAt == "" {
			t.Errorf("status surface incomplete: %+v", e)
		}
	}
	if _, ok := byID[h1.NoteID]; !ok {
		t.Errorf("handoff one missing")
	}
	if _, ok := byID[h2.NoteID]; !ok {
		t.Errorf("handoff two missing")
	}
	if _, ok := byID[auto.ID]; !ok {
		t.Errorf("auto handoff missing")
	}
	if !byID[auto.ID].AutoCaptured {
		t.Errorf("auto-captured badge missing on %+v", byID[auto.ID])
	}
	if byID[h1.NoteID].AutoCaptured || byID[h2.NoteID].AutoCaptured {
		t.Errorf("manual handoffs must not be flagged auto")
	}

	// Content-free: the ring must never carry note bodies or summaries.
	blob := fmt.Sprintf("%+v", got)
	for _, leaked := range []string{"plain body", "decoy body", "auto body", "first", "second"} {
		if strings.Contains(blob, leaked) {
			t.Errorf("memory ring leaked note content %q", leaked)
		}
	}

	// Newest first with same-second ties broken by id DESC: limit=2 must
	// return the two newest handoffs (auto, then h2).
	top2 := svc.ListRecentHandoffs(2)
	if len(top2) != 2 || top2[0].NoteID != auto.ID || top2[1].NoteID != h2.NoteID {
		t.Fatalf("limit/ordering: got %+v, want [%d %d]", top2, auto.ID, h2.NoteID)
	}

	// An oversized ask is clamped, not an error.
	if got := svc.ListRecentHandoffs(1000); len(got) != 3 {
		t.Fatalf("clamped ask returned %d entries, want 3", len(got))
	}
}
