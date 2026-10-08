package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

// snapshotSchema records every object sqlite knows about, so a migration can be
// proven reversible rather than merely "not crashing".
func snapshotSchema(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT type, name, COALESCE(sql, '') FROM sqlite_master ORDER BY type, name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var typ, name, sql string
		if err := rows.Scan(&typ, &name, &sql); err != nil {
			t.Fatal(err)
		}
		out = append(out, typ+"|"+name+"|"+sql)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// W1's acceptance bar from ADR-0014: "迁移必须可逆并带测试".
//
// Reversible here means something stronger than "there is a down script": the
// wiki layer adds only new objects and touches no pre-existing table, so after
// DropWikiSchema the schema must be IDENTICAL to before. Anything less would let
// a future "small" ALTER into project_notes hide inside this migration.
func TestWikiMigration_IsReversibleByConstruction(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "rev.db")

	db1, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// v16 has already run, so capture "before" at v15 by dropping the layer, then
	// re-apply and compare. This also proves EnsureWikiSchema is replay-safe.
	before := func() []string {
		if err := DropWikiSchema(db1); err != nil {
			t.Fatalf("drop: %v", err)
		}
		return snapshotSchema(t, db1)
	}()
	if err := EnsureWikiSchema(db1); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	dirty := snapshotSchema(t, db1)
	if reflect.DeepEqual(before, dirty) {
		t.Fatal("schema unchanged after EnsureWikiSchema — the layer created nothing, so the comparison is vacuous")
	}
	if err := DropWikiSchema(db1); err != nil {
		t.Fatalf("drop again: %v", err)
	}
	after := snapshotSchema(t, db1)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("wiki layer is not reversible:\n before=%d objects\n after =%d objects\n diff:\n  %v\n  %v",
			len(before), len(after), minus(before, after), minus(after, before))
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	// A fresh database must arrive with the layer present (migration ran, not just
	// a helper someone might forget to call).
	db2, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db2.Close() }()
	if _, err := db2.Query("SELECT 1 FROM wiki_pages LIMIT 1"); err != nil {
		t.Fatalf("fresh InitDB has no wiki_pages: %v", err)
	}
}

func minus(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	var out []string
	for _, x := range a {
		if !set[x] {
			out = append(out, x)
		}
	}
	return out
}

func TestWikiPages_KindIsEnforced(t *testing.T) {
	db := setupTestDB(t)

	if _, err := CreateWikiPage(db, "nonsense", "orphan-page", "Orphan", 0, ""); err == nil {
		t.Fatal("an unknown kind was accepted by CreateWikiPage")
	}
	// The CHECK must hold even if someone bypasses the Go validator, otherwise the
	// constraint is decoration and the enum lives in one place only.
	if _, err := db.Exec(
		`INSERT INTO wiki_pages(slug, title, kind) VALUES ('raw-insert','Raw','who-knows')`); err == nil {
		t.Error("the CHECK constraint did not reject an unknown kind at the SQL level")
	}
	for _, k := range []string{WikiKindEntity, WikiKindConcept, WikiKindSource, WikiKindSynthesis, WikiKindQuery} {
		if _, err := CreateWikiPage(db, k, "page-"+k, "Page "+k, 0, "body"); err != nil {
			t.Errorf("kind %q rejected: %v", k, err)
		}
	}
	counts, err := WikiPageCount(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 5 {
		t.Errorf("kind histogram = %v, want five distinct kinds", counts)
	}
}

// The slug is the wikilink address, so uniqueness is the one thing W2/W3 will
// depend on absolutely — and the reason it is globally unique rather than per
// project (a per-project index would leave a NULL hole for cross-project pages).
func TestWikiPages_SlugIsGloballyUniqueAndDistinguishable(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "slugproj")

	if _, err := CreateWikiPage(db, WikiKindEntity, "payment-gateway", "Payment Gateway", pid, "first"); err != nil {
		t.Fatal(err)
	}
	_, err := CreateWikiPage(db, WikiKindConcept, "payment-gateway", "Duplicate", 0, "second")
	if err == nil {
		t.Fatal("a duplicate slug was accepted")
	}
	if !errors.Is(err, ErrWikiSlugTaken) {
		t.Errorf("duplicate slug error = %v, want it to wrap ErrWikiSlugTaken so callers can tell conflict from failure", err)
	}

	// Normalization must be deterministic and shared with link resolution.
	cases := map[string]string{
		"Payment Gateway":        "payment-gateway",
		"  Multi   Space  ":      "multi-space",
		"Under_score.dot":        "under-score-dot",
		"中文 标题":                  "中文-标题",
		"trailing----":           "trailing",
		"CamelCase Name":         "camelcase-name",
		"---leading and tail---": "leading-and-tail",
	}
	for in, want := range cases {
		if got := NormalizeWikiSlug(in); got != want {
			t.Errorf("NormalizeWikiSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

// The whole point of the layer over an adjacency list is the backlink direction.
func TestWikiLinks_AreBidirectionalAndIdempotent(t *testing.T) {
	db := setupTestDB(t)

	a, err := CreateWikiPage(db, WikiKindEntity, "auth-module", "Auth Module", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateWikiPage(db, WikiKindConcept, "session-timeout", "Session Timeout", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, b.ID, a.ID, "explains"); err != nil {
		t.Fatal(err)
	}
	// Same triple twice: a re-run of the same ingest must not stack edges.
	if err := LinkWikiPages(db, b.ID, a.ID, "explains"); err != nil {
		t.Fatalf("re-link was rejected: %v", err)
	}
	// A different relation between the same pair IS a different edge.
	if err := LinkWikiPages(db, b.ID, a.ID, "depends-on"); err != nil {
		t.Fatal(err)
	}

	out, err := WikiEdgesFrom(db, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("outgoing edges = %d, want 2 (one per relation)", len(out))
	}
	back, err := WikiEdgesTo(db, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 {
		t.Fatalf("backlinks = %d, want 2", len(back))
	}
	for _, e := range back {
		if !e.Inbound {
			t.Error("backlink rows are not marked inbound")
		}
		if e.PageID != b.ID {
			t.Errorf("backlink points at %d, want %d", e.PageID, b.ID)
		}
	}

	if err := UnlinkWikiPages(db, b.ID, a.ID, "explains"); err != nil {
		t.Fatal(err)
	}
	after, err := WikiEdgesTo(db, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].Relation != "depends-on" {
		t.Errorf("after unlink: %+v, want only depends-on", after)
	}
}

func TestWikiLinks_RejectSelfLoopsAndDanglingEndpoints(t *testing.T) {
	db := setupTestDB(t)
	a, err := CreateWikiPage(db, WikiKindEntity, "solo", "Solo", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, a.ID, a.ID, "ref"); err == nil {
		t.Error("a page was allowed to link to itself")
	}
	if err := LinkWikiPages(db, a.ID, 9999, "ref"); err == nil {
		t.Error("a link to a nonexistent page was accepted — dangling edges would make W4 lint meaningless")
	}
}

// Deleting a page must not leave edges or note attachments behind: orphan rows in
// a link table look harmless until they surface as a page with a ghost neighbor.
func TestWikiPages_DeleteCascades(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "cascadeproj")
	note, err := CreateNoteEx(db, pid, "note", "body", "", "knowledge", "manual")
	if err != nil {
		t.Fatal(err)
	}
	from, err := CreateWikiPage(db, WikiKindSource, "src-a", "Src A", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	to, err := CreateWikiPage(db, WikiKindSynthesis, "syn-b", "Syn B", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, from.ID, to.ID, "feeds"); err != nil {
		t.Fatal(err)
	}
	if err := AttachNoteToPage(db, note.ID, from.ID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteWikiPage(db, from.ID); err != nil {
		t.Fatal(err)
	}

	var edges, attachments int
	if err := db.QueryRow("SELECT COUNT(*) FROM page_links").Scan(&edges); err != nil {
		t.Fatal(err)
	}
	if edges != 0 {
		t.Errorf("%d edge(s) survived deleting a page", edges)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM note_pages").Scan(&attachments); err != nil {
		t.Fatal(err)
	}
	if attachments != 0 {
		t.Errorf("%d note attachment(s) survived deleting a page", attachments)
	}
}

// The second structural gap project_notes never closed: a note could name a
// project but never the repository inside it.
func TestNoteRepositoryAssociation(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "repos")
	repoPath := "/tmp/repos/sub"
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertRepositoryTx(tx, repoPath, pid); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var repoID int64
	if err := db.QueryRow("SELECT id FROM repositories WHERE path = ?", repoPath).Scan(&repoID); err != nil {
		t.Fatal(err)
	}
	note, err := CreateNoteEx(db, pid, "note", "body", "", "other", "manual")
	if err != nil {
		t.Fatal(err)
	}

	if err := AttachNoteToRepository(db, note.ID, repoID); err != nil {
		t.Fatal(err)
	}
	if err := AttachNoteToRepository(db, note.ID, repoID); err != nil {
		t.Fatalf("re-attach rejected (must be idempotent): %v", err)
	}
	ids, err := RepositoriesForNote(db, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != repoID {
		t.Errorf("RepositoriesForNote = %v, want [%d]", ids, repoID)
	}
	back, err := NotesForRepository(db, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 || back[0] != note.ID {
		t.Errorf("NotesForRepository = %v, want [%d]", back, note.ID)
	}
	if err := DetachNoteFromRepository(db, note.ID, repoID); err != nil {
		t.Fatal(err)
	}
	if got, _ := RepositoriesForNote(db, note.ID); len(got) != 0 {
		t.Errorf("after detach: %v, want empty", got)
	}
	// Deleting the note must clean up its side of the association.
	if err := AttachNoteToRepository(db, note.ID, repoID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteNote(db, note.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow("SELECT COUNT(*) FROM note_repositories").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d note/repository row(s) survived deleting the note", left)
	}
}

// A page attached to a project must disappear with the project, the same cascade
// the notes it was compiled from already follow.
func TestWikiPages_ProjectDeleteCascades(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "cascade-owner")
	if _, err := CreateWikiPage(db, WikiKindEntity, "owned-page", "Owned", pid, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM projects WHERE id = ?", pid); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM wiki_pages WHERE project_id = ?", pid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d page(s) survived deleting their project", n)
	}
}
