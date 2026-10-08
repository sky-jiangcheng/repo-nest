package service

import (
	"strings"
	"testing"

	"repo-nest/internal/db"
)

func TestClassifyNoteLayer(t *testing.T) {
	cases := []struct {
		source, kind string
		want         MemoryLayer
		why          string
	}{
		{"codex", "log", LayerTranscripts, "raw rollout transcript"},
		{"opencode", "log", LayerTranscripts, "session summary import"},
		{"cursor", "log", LayerTranscripts, "session import"},
		{"import:cursor", "log", LayerTranscripts, "the import:<source> spelling is the same class"},
		{"manual", "log", LayerTranscripts, "kind log is session-shaped even when hand-written"},
		{"claude", "knowledge", LayerNotes, "condensed memory file, not a transcript"},
		{"openclaw", "knowledge", LayerNotes, "global memory file"},
		{"hermes", "knowledge", LayerNotes, "global memory file"},
		{"manual", "knowledge", LayerNotes, "curated note"},
		{"manual", "idea", LayerNotes, "curated note"},
		{"ai", "knowledge", LayerNotes, "a filed Q&A answer is curated"},
		{"manual", "", LayerNotes, "an unset kind must not fall into the raw stratum"},
		{"CODEX", "log", LayerTranscripts, "source matching is case-insensitive"},
	}
	for _, tc := range cases {
		if got := ClassifyNoteLayer(tc.source, tc.kind); got != tc.want {
			t.Errorf("ClassifyNoteLayer(%q,%q) = %s, want %s (%s)", tc.source, tc.kind, got, tc.want, tc.why)
		}
	}
}

func layerCorpus(t *testing.T, svc *Service) (projectID int64) {
	t.Helper()
	projectID = seedProject(t, svc.db, "layers", "/tmp/layers")
	add := func(title, content, kind, source string) {
		t.Helper()
		if _, err := db.CreateNoteEx(svc.db, projectID, title, content, "", kind, source); err != nil {
			t.Fatal(err)
		}
	}
	add("Payment retry policy", "The gateway retries three times with exponential backoff.",
		"knowledge", "manual")
	add("Session rollout excerpt", "User asked about gateway backoff behavior in a long session transcript body.",
		"log", "codex")
	return projectID
}

// Orientation-only means exactly that: an empty query must not trigger per-fact
// retrieval at all. That is the difference between "cheap, always-available bearings"
// and "run a search nobody asked for".
func TestBuildLayeredMemory_OrientationOnlyRetrievesNothing(t *testing.T) {
	svc, _ := setupService(t)
	pid := layerCorpus(t, svc)

	m := svc.BuildLayeredMemory(pid, "", LayerBudget{}, false)
	if m.FellBack {
		t.Error("orientation-only assembly claims it fell back to per-fact retrieval")
	}
	for _, blk := range m.Blocks {
		if blk.Layer == LayerNotes || blk.Layer == LayerTranscripts {
			t.Errorf("retrieval layer %q present without a query", blk.Layer)
		}
	}
	if len(m.Blocks) == 0 || m.Blocks[0].Layer != LayerProjectScenario {
		t.Errorf("L2 project scenario missing from an orientation call: %+v", m.Layers())
	}
}

// L0 is the noisiest, largest stratum and ADR-0014 keeps transcripts behind an
// explicit opt-in, so the default must exclude them even when they match.
func TestBuildLayeredMemory_TranscriptsAreOptIn(t *testing.T) {
	svc, _ := setupService(t)
	pid := layerCorpus(t, svc)

	q := "gateway backoff"
	off := svc.BuildLayeredMemory(pid, q, LayerBudget{}, false)
	on := svc.BuildLayeredMemory(pid, q, LayerBudget{}, true)

	var notesOff, notesOn, transcripts int
	for _, blk := range off.Blocks {
		if blk.Layer == LayerNotes {
			notesOff += blk.Items
		}
	}
	for _, blk := range on.Blocks {
		switch blk.Layer {
		case LayerNotes:
			notesOn += blk.Items
		case LayerTranscripts:
			transcripts += blk.Items
		}
	}
	if transcripts == 0 {
		t.Fatal("the fixture transcript matched nothing even when requested; test is vacuous")
	}
	for _, blk := range off.Blocks {
		if blk.Layer == LayerTranscripts {
			t.Error("raw transcripts included without opting in")
		}
	}
	if notesOn != notesOff {
		t.Errorf("adding L0 changed L1 output (%d -> %d): layers must not compete for one budget",
			notesOff, notesOn)
	}
}

// Order is the product: stable orientation before volatile specifics.
func TestBuildLayeredMemory_OrderIsStableBeforeVolatile(t *testing.T) {
	svc, _ := setupService(t)
	pid := layerCorpus(t, svc)
	if _, err := db.CreateWikiPageAs(svc.db, db.WikiKindSynthesis, "cross-project-conventions",
		"Cross-project conventions", 0, "All services in this fleet log structured JSON.",
		db.WikiStatusApproved, "manual"); err != nil {
		t.Fatal(err)
	}
	m := svc.BuildLayeredMemory(pid, "gateway backoff", LayerBudget{}, true)
	if len(m.Blocks) < 3 {
		t.Fatalf("expected at least L3+L2+L1, got %+v", m.Layers())
	}
	if m.Blocks[0].Layer != LayerGlobalProfile {
		t.Errorf("first block = %s, want L3 global profile", m.Blocks[0].Layer)
	}
	idx := map[MemoryLayer]int{}
	for i, blk := range m.Blocks {
		idx[blk.Layer] = i
	}
	if idx[LayerGlobalProfile] > idx[LayerProjectScenario] || idx[LayerProjectScenario] > idx[LayerNotes] {
		t.Errorf("layer order broken: %+v", m.Layers())
	}
	if idx[LayerNotes] > idx[LayerTranscripts] {
		t.Errorf("transcripts must come last: %+v", m.Layers())
	}
	rendered := m.Render()
	if !strings.Contains(rendered, "L3-global") || !strings.Contains(rendered, "L0-transcripts") {
		t.Error("rendered prompt must label layers, so a model can weigh them differently")
	}
}

// Per-layer caps, not one shared cap: a single global budget lets a chatty L0
// crowd out the bearings the whole design exists to provide.
func TestBuildLayeredMemory_CapsArePerLayer(t *testing.T) {
	svc, _ := setupService(t)
	pid := layerCorpus(t, svc)
	for i := 0; i < 3; i++ {
		if _, err := db.CreateWikiPageAs(svc.db, db.WikiKindConcept, "conv-"+string(rune('a'+i)),
			"Convention "+string(rune('a'+i)), 0,
			"A deliberately long cross-project convention entry so the character cap has something to cut, and it keeps going on and on to make sure of it.",
			db.WikiStatusApproved, "manual"); err != nil {
			t.Fatal(err)
		}
	}
	m := svc.BuildLayeredMemory(pid, "gateway backoff", LayerBudget{GlobalChars: 120}, true)
	var global LayerBlock
	for _, blk := range m.Blocks {
		if blk.Layer == LayerGlobalProfile {
			global = blk
		}
	}
	if global.Items == 0 {
		t.Fatal("L3 vanished under a tight cap; the cap must trim, not mute the layer")
	}
	if len([]rune(global.Text)) > 140 {
		t.Errorf("L3 text is %d runes over a 120 cap", len([]rune(global.Text)))
	}
	if global.Omitted == 0 {
		t.Error("trimmed items must be reported, not silently dropped")
	}
	// L2 survives a tight L3 cap — the layers are independent budgets.
	found := false
	for _, blk := range m.Blocks {
		if blk.Layer == LayerProjectScenario && blk.Items > 0 {
			found = true
		}
	}
	if !found {
		t.Error("L2 dropped because L3 was capped; budgets leaked across layers")
	}
}

// An unreviewed page must not steer an answer: L3 reads approved-only, same rule
// as retrieval (ADR-0015 决策 1).
func TestBuildLayeredMemory_ExcludesPendingFromGlobalProfile(t *testing.T) {
	svc, _ := setupService(t)
	pid := layerCorpus(t, svc)
	if _, err := db.CreateWikiPageAs(svc.db, db.WikiKindConcept, "unreviewed-thing",
		"Unreviewed thing", 0, "Model-written content that nobody has approved yet, long enough to not be trimmed away by accident.",
		db.WikiStatusPending, compileSourceTag); err != nil {
		t.Fatal(err)
	}
	m := svc.BuildLayeredMemory(pid, "", LayerBudget{}, false)
	if strings.Contains(m.Render(), "Unreviewed thing") {
		t.Error("a pending page entered the orientation context")
	}
	// Another project's page must not appear either: global means no owner, not
	// "any project".
	other := seedProject(t, svc.db, "other-layers", "/tmp/other-layers")
	if _, err := db.CreateWikiPageAs(svc.db, db.WikiKindConcept, "owned-elsewhere",
		"Owned Elsewhere", other, "A page that belongs to a different project, long enough to survive the caps in this test.",
		db.WikiStatusApproved, "manual"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(svc.BuildLayeredMemory(pid, "", LayerBudget{}, false).Render(), "Owned Elsewhere") {
		t.Error("another project's page leaked into this project's orientation")
	}
}

// Nothing configured must be a clean empty, not a panic or a fake block.
func TestBuildLayeredMemory_EmptyStore(t *testing.T) {
	svc, _ := setupService(t)
	m := svc.BuildLayeredMemory(0, "anything", LayerBudget{}, true)
	if len(m.Blocks) != 0 || m.Chars != 0 || m.Render() != "" {
		t.Errorf("empty store produced %+v", m)
	}
	// Global profile with no project asked for is still legitimate (projectID 0
	// means "no project scenario", not "everything").
	if _, err := db.CreateWikiPageAs(svc.db, db.WikiKindSynthesis, "fleet", "Fleet", 0,
		"A global profile line long enough to be kept by the default character budget in this test.",
		db.WikiStatusApproved, "manual"); err != nil {
		t.Fatal(err)
	}
	m2 := svc.BuildLayeredMemory(0, "", LayerBudget{}, false)
	if len(m2.Blocks) != 1 || m2.Blocks[0].Layer != LayerGlobalProfile {
		t.Errorf("global-only assembly = %+v", m2.Layers())
	}
}
