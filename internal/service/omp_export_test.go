package service

import (
	"encoding/json"
	"strings"
	"testing"

	"repotory/internal/db"
)

func TestExportMemoryJSON(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "ompproj", "/tmp/ompproj")
	if _, err := db.CreateNoteEx(svc.db, pid, "架构事实", "sqlite 是主库", "db", "knowledge", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateNoteEx(svc.db, pid, "会话交接", "本次改了 X", "handoff", "log", "mcp"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateNoteEx(svc.db, pid, "想法", "以后支持 Y", "", "idea", "manual"); err != nil {
		t.Fatal(err)
	}

	raw := svc.ExportMemoryJSON(0)
	var out []OMPMemory
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, raw)
	}
	if len(out) != 3 {
		t.Fatalf("got %d memories, want 3", len(out))
	}

	byType := map[string]int{}
	for _, m := range out {
		byType[m.Type]++
		if m.Source.Tool != "repotory" {
			t.Errorf("source.tool = %q, want repotory", m.Source.Tool)
		}
		if !strings.HasPrefix(m.ID, "urn:repotory:note:") {
			t.Errorf("bad id %q", m.ID)
		}
	}
	// knowledge + idea mapping: semantic(architecture) + procedural(idea); handoff tag -> episodic.
	if byType["episodic"] != 1 || byType["procedural"] != 1 || byType["semantic"] != 1 {
		t.Errorf("type distribution = %v, want 1/1/1 (episodic/procedural/semantic)", byType)
	}
	// handoff note's content should carry title + body.
	var found bool
	for _, m := range out {
		if strings.Contains(m.Content, "会话交接") {
			found = true
		}
	}
	if !found {
		t.Error("handoff note content not present in export")
	}
}
