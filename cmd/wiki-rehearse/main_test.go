package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	"repo-nest/internal/db"
)

func TestReconcile_Pure(t *testing.T) {
	type tc struct {
		name        string
		before      []edge
		after       []edge
		wantPass    bool
		wantCollapse int
		wantLost    int
		wantExtra   int
		wantOffVocab int
	}
	cases := []tc{
		{
			name:     "identity: already vocabulary",
			before:   []edge{{From: 1, To: 2, Relation: "ref"}, {From: 2, To: 3, Relation: "depends"}},
			after:    []edge{{From: 1, To: 2, Relation: "ref"}, {From: 2, To: 3, Relation: "depends"}},
			wantPass: true,
		},
		{
			name: "collapse: parallel legacy edges fold to one ref",
			before: []edge{
				{From: 1, To: 2, Relation: "cites"},
				{From: 1, To: 2, Relation: "compiled-from"}, // same pair, different legacy value
				{From: 1, To: 3, Relation: "wildcard"},
				{From: 2, To: 3, Relation: "depends"},
			},
			after: []edge{
				{From: 1, To: 2, Relation: "ref"},
				{From: 1, To: 3, Relation: "ref"},
				{From: 2, To: 3, Relation: "depends"},
			},
			wantPass:     true,
			wantCollapse: 1, // 4 before -> 3 distinct derived keys
		},
		{
			name:     "lost: an expected edge is missing from the result",
			before:   []edge{{From: 2, To: 3, Relation: "depends"}},
			after:    []edge{},
			wantPass: false,
			wantLost: 1,
		},
		{
			name:     "extra: a fabricated edge appears",
			before:   []edge{{From: 1, To: 2, Relation: "ref"}},
			after:    []edge{{From: 1, To: 2, Relation: "ref"}, {From: 5, To: 6, Relation: "ref"}},
			wantPass: false,
			wantExtra: 1,
		},
		{
			name:     "off-vocabulary survived (migration failed to normalize)",
			before:   []edge{{From: 1, To: 2, Relation: "nonsense"}},
			after:    []edge{{From: 1, To: 2, Relation: "nonsense"}},
			wantPass: false,
			wantLost: 1, wantExtra: 1, wantOffVocab: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := reconcile(c.before, c.after)
			if r.Pass != c.wantPass {
				t.Errorf("Pass = %v, want %v (lost=%v extra=%v off=%v)", r.Pass, c.wantPass, r.Lost, r.Extra, r.OffVocabulary)
			}
			if r.Collapsed != c.wantCollapse {
				t.Errorf("Collapsed = %d, want %d", r.Collapsed, c.wantCollapse)
			}
			if len(r.Lost) != c.wantLost {
				t.Errorf("lost = %v, want %d", r.Lost, c.wantLost)
			}
			if len(r.Extra) != c.wantExtra {
				t.Errorf("extra = %v, want %d", r.Extra, c.wantExtra)
			}
			if len(r.OffVocabulary) != c.wantOffVocab {
				t.Errorf("offVocab = %v, want %d", r.OffVocabulary, c.wantOffVocab)
			}
		})
	}
}

const oldShapeDDL = `CREATE TABLE page_links (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	from_page_id INTEGER NOT NULL,
	to_page_id INTEGER NOT NULL,
	relation TEXT NOT NULL DEFAULT 'ref',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	CHECK (from_page_id <> to_page_id),
	UNIQUE (from_page_id, to_page_id, relation),
	FOREIGN KEY (from_page_id) REFERENCES wiki_pages(id) ON DELETE CASCADE,
	FOREIGN KEY (to_page_id) REFERENCES wiki_pages(id) ON DELETE CASCADE
)`

// buildPreV21DB fabricates a database exactly as a v20 install would look: a real
// wiki layer minus the v21 CHECK, stamped at schema_version 20, with legacy
// free-text relations including two parallel edges that must collapse.
func buildPreV21DB(t *testing.T, path string) {
	t.Helper()
	d, err := db.InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	a, _ := db.CreateWikiPage(d, db.WikiKindEntity, "a", "A", 0, "body padding to stay above any content floor in this harness")
	b, _ := db.CreateWikiPage(d, db.WikiKindEntity, "b", "B", 0, "body padding to stay above any content floor in this harness")
	c, _ := db.CreateWikiPage(d, db.WikiKindEntity, "c", "C", 0, "body padding to stay above any content floor in this harness")

	if _, err := d.Exec(`DROP TABLE page_links`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(oldShapeDDL); err != nil {
		t.Fatal(err)
	}
	seed := []struct {
		f, t int64
		rel  string
	}{
		{a.ID, b.ID, "cites"},
		{a.ID, b.ID, "compiled-from"}, // folds to ref, collides with the row above
		{a.ID, c.ID, "wildcard-value"}, // unknown -> ref
		{b.ID, c.ID, "depends"},        // already canonical, must survive untouched
	}
	for _, s := range seed {
		if _, err := d.Exec(`INSERT INTO page_links(from_page_id,to_page_id,relation) VALUES(?,?,?)`, s.f, s.t, s.rel); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Exec(`UPDATE app_config SET value = '20' WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
}

// The whole rehearsal on a synthetic pre-v21 database: it must pass, migrate to
// v21, and — the load-bearing safety claim — leave the SOURCE byte-unchanged and
// read-only.
func TestRehearse_Synthetic_PreV21(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dashboard.db")
	buildPreV21DB(t, src)

	rep, hadNoTable, err := rehearse(src, filepath.Join(dir, "rehearse-copy.db"))
	if err != nil {
		t.Fatalf("rehearse: %v", err)
	}
	if hadNoTable {
		t.Fatal("hadNoTable true, but the synthetic db had page_links")
	}
	if !rep.Pass {
		t.Fatalf("rehearsal did not pass: lost=%v extra=%v off=%v", rep.Lost, rep.Extra, rep.OffVocabulary)
	}
	if rep.VersionBefore != 20 {
		t.Errorf("VersionBefore = %d, want 20", rep.VersionBefore)
	}
	if rep.VersionAfter != 21 {
		t.Errorf("VersionAfter = %d, want 21 (v21 migration target)", rep.VersionAfter)
	}
	if rep.BeforeCount != 4 || rep.AfterCount != 3 {
		t.Errorf("edges %d->%d, want 4->3 (two a->b folded)", rep.BeforeCount, rep.AfterCount)
	}
	if rep.Collapsed != 1 {
		t.Errorf("Collapsed = %d, want 1", rep.Collapsed)
	}
	for rel := range rep.AfterHistogram {
		if !db.ValidRelation(rel) {
			t.Errorf("after histogram has off-vocabulary relation %q", rel)
		}
	}

	// Safety: the source must be UNTOUCHED — still at v20 with its legacy values.
	chk, err := sql.Open("sqlite", "file:"+filepath.ToSlash(src)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer chk.Close()
	if v := readSchemaVersion(chk); v != 20 {
		t.Errorf("source was mutated: schema_version now %d, want 20", v)
	}
	before, err := dumpEdges(chk)
	if err != nil {
		t.Fatal(err)
	}
	sawLegacy := false
	for _, e := range before {
		if e.Relation == "cites" || e.Relation == "compiled-from" || e.Relation == "wildcard-value" {
			sawLegacy = true
		}
	}
	if !sawLegacy {
		t.Error("source lost its legacy relation values — the rehearsal must not write to it")
	}
	if len(before) != 4 {
		t.Errorf("source edge count = %d, want 4 (unchanged)", len(before))
	}
}
