package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// ADR-0021 晋升条件 1 + 2 for the page-version layer.
//
// The interesting cases here are not "does the table exist": they are the two
// failure modes the design was chosen to avoid — a snapshot table that silently
// stops the user from deleting a project (the FK/CASCADE interaction documented
// in wiki_versions.go), and an approve transition that files a new version every
// time the button is pressed.

func countPageVersions(t *testing.T, database *sql.DB, pageID int64) int {
	t.Helper()
	var n int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM page_versions WHERE page_id = ?", pageID).Scan(&n); err != nil {
		t.Fatalf("count page versions: %v", err)
	}
	return n
}

func mustExec(t *testing.T, database *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := database.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

// 决策 1 + 2: only approved content is frozen, and only on the transitions that
// matter. Pending drafts and rejected tombstones have no "conclusion history".
func TestPageVersions_SnapshotsOnlyApprovedStates(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "versions")

	pending, err := CreateWikiPageAs(database, WikiKindConcept, "v-pending", "Pending", proj,
		"draft body", WikiStatusPending, "wiki-compile")
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	if n := countPageVersions(t, database, pending.ID); n != 0 {
		t.Fatalf("a pending page must have no snapshot, got %d", n)
	}

	// Editing a draft must not file versions either — that would make the history
	// table a function of how much drafting happened.
	if err := UpdateWikiPage(database, pending.ID, "Pending", WikiKindConcept, "draft v2"); err != nil {
		t.Fatalf("update pending: %v", err)
	}
	if n := countPageVersions(t, database, pending.ID); n != 0 {
		t.Fatalf("editing a pending page must not snapshot, got %d", n)
	}

	// Approving freezes exactly the content that was approved.
	if err := SetWikiPageStatus(database, pending.ID, WikiStatusApproved); err != nil {
		t.Fatalf("approve: %v", err)
	}
	vers, err := ListPageVersions(database, pending.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(vers) != 1 {
		t.Fatalf("approve must file exactly one snapshot, got %d", len(vers))
	}
	if vers[0].Content != "draft v2" || vers[0].Title != "Pending" || vers[0].Kind != WikiKindConcept {
		t.Fatalf("snapshot must reproduce the approved state verbatim: %+v", vers[0])
	}

	// A second approve (the review button pressed twice, or a replayed job) must be
	// a no-op: the WHEN clause needs a status or content CHANGE, not just an UPDATE
	// that bumps updated_at.
	if err := SetWikiPageStatus(database, pending.ID, WikiStatusApproved); err != nil {
		t.Fatalf("re-approve: %v", err)
	}
	if n := countPageVersions(t, database, pending.ID); n != 1 {
		t.Fatalf("re-approving must not duplicate the snapshot, got %d", n)
	}

	// Mutating approved content files the new state, so the previous entry plus the
	// live row are both reconstructable.
	if err := UpdateWikiPage(database, pending.ID, "Pending", WikiKindConcept, "edited after approval"); err != nil {
		t.Fatalf("update approved: %v", err)
	}
	vers, err = ListPageVersions(database, pending.ID)
	if err != nil {
		t.Fatalf("list after edit: %v", err)
	}
	if len(vers) != 2 {
		t.Fatalf("editing an approved page must file a snapshot, got %d", len(vers))
	}
	if vers[0].Content != "edited after approval" {
		t.Fatalf("ListPageVersions must be newest first, oldest first entry: %q", vers[0].Content)
	}

	// Rejecting (which replaces the body with a tombstone) must NOT file a version:
	// new.status is 'rejected', so the trigger's WHEN clause is false. A merge
	// (ADR-0019 决策 5) lands here on purpose.
	if err := SetWikiPageStatus(database, pending.ID, WikiStatusRejected); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if n := countPageVersions(t, database, pending.ID); n != 2 {
		t.Fatalf("rejecting must not file a snapshot, got %d", n)
	}
}

// 晋升条件 1: the backfill must reconcile one snapshot per approved page, byte for
// byte, and must not invent history for pages that already have it.
func TestPageVersions_BackfillReconcilesApprovedPages(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "backfill")

	// A page BORN approved (human-authored, or filed by W2) never passes through an
	// UPDATE, so the trigger cannot see it — the backfill is what gives it a
	// baseline. This is why EnsurePageVersions is re-asserted from InitDB rather
	// than being a one-shot migration.
	born, err := CreateWikiPage(database, WikiKindEntity, "bf-born", "Born approved", proj, "born body")
	if err != nil {
		t.Fatalf("create born: %v", err)
	}
	if n := countPageVersions(t, database, born.ID); n != 0 {
		t.Fatalf("insert alone must not snapshot, got %d", n)
	}
	if err := EnsurePageVersions(database); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	vers, err := ListPageVersions(database, born.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(vers) != 1 {
		t.Fatalf("backfill must file exactly one baseline, got %d", len(vers))
	}
	if vers[0].Content != "born body" || vers[0].Title != "Born approved" {
		t.Fatalf("backfilled snapshot must equal the live page: %+v", vers[0])
	}

	// Re-running the whole ensure (InitDB does, on every open) must not touch it.
	if err := EnsurePageVersions(database); err != nil {
		t.Fatalf("ensure again: %v", err)
	}
	if n := countPageVersions(t, database, born.ID); n != 1 {
		t.Fatalf("backfill must be per-page idempotent, got %d", n)
	}

	// Pending and rejected pages stay out of the ledger entirely.
	if _, err := CreateWikiPageAs(database, WikiKindConcept, "bf-pending", "P", proj,
		"x", WikiStatusPending, "wiki-compile"); err != nil {
		t.Fatalf("create pending: %v", err)
	}
	if err := EnsurePageVersions(database); err != nil {
		t.Fatalf("ensure third: %v", err)
	}
	var pendingSnaps int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM page_versions pv JOIN wiki_pages w ON w.id = pv.page_id
		  WHERE w.status <> 'approved'`).Scan(&pendingSnaps); err != nil {
		t.Fatalf("count non-approved snapshots: %v", err)
	}
	if pendingSnaps != 0 {
		t.Fatalf("only approved pages may be snapshotted, got %d rows", pendingSnaps)
	}
}

// 决策 3: append-only is a structural fact. The error text is asserted because a
// generic "constraint failed" would also be produced by, say, an accidental FK —
// and this test's whole purpose is to prove it is the GUARD that fired.
func TestPageVersions_IsAppendOnly(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "appendonly")
	page, err := CreateWikiPage(database, WikiKindConcept, "ao-1", "Page", proj, "content")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := EnsurePageVersions(database); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	vers, err := ListPageVersions(database, page.ID)
	if err != nil || len(vers) != 1 {
		t.Fatalf("expected one snapshot to guard, got %d (%v)", len(vers), err)
	}

	if _, err := database.Exec("UPDATE page_versions SET content = 'rewritten history' WHERE id = ?", vers[0].ID); err == nil {
		t.Fatal("UPDATE on page_versions must be refused by the append-only trigger")
	} else if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected the append-only guard to reject the update, got: %v", err)
	}
	if _, err := database.Exec("DELETE FROM page_versions WHERE id = ?", vers[0].ID); err == nil {
		t.Fatal("DELETE on page_versions must be refused by the append-only trigger")
	} else if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected the append-only guard to reject the delete, got: %v", err)
	}

	// The live page still moves freely — the guard is on the history table only.
	if err := UpdateWikiPage(database, page.ID, "Page", WikiKindConcept, "v2"); err != nil {
		t.Fatalf("updating the live page must not be blocked: %v", err)
	}
	if n := countPageVersions(t, database, page.ID); n != 2 {
		t.Fatalf("expected the edit to file a second snapshot, got %d", n)
	}
}

// The trap that decided the schema: SQLite fires row triggers for foreign-key
// CASCADE actions, so `page_versions.page_id REFERENCES wiki_pages(id) ON DELETE
// CASCADE` plus the BEFORE DELETE guard would make deleting a project fail with
// "append-only". An audit table that blocks the user's normal housekeeping is
// worse than an orphaned audit row, so there is no FK — and this test is the
// proof that the choice holds rather than a comment claiming it does.
func TestPageVersions_ProjectDeleteIsNotBlockedByAppendOnly(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "cascade")
	page, err := CreateWikiPage(database, WikiKindConcept, "cas-1", "Page", proj, "content")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := EnsurePageVersions(database); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if n := countPageVersions(t, database, page.ID); n != 1 {
		t.Fatalf("expected a baseline snapshot before the delete, got %d", n)
	}

	mustExec(t, database, "DELETE FROM projects WHERE id = ?", proj)

	// Deleting the project cascades over the live pages...
	var pages int
	if err := database.QueryRow("SELECT COUNT(*) FROM wiki_pages WHERE id = ?", page.ID).Scan(&pages); err != nil {
		t.Fatalf("count pages: %v", err)
	}
	if pages != 0 {
		t.Fatal("wiki_pages row should have cascaded away with its project")
	}
	// ...and leaves the audit rows exactly where they were.
	if n := countPageVersions(t, database, page.ID); n != 1 {
		t.Fatalf("the snapshot must survive its page's deletion, got %d rows", n)
	}
	// Reading the history of a page that no longer exists is the whole point.
	vers, err := ListPageVersions(database, page.ID)
	if err != nil || len(vers) != 1 || vers[0].Content != "content" {
		t.Fatalf("orphaned history must stay readable, got %+v (%v)", vers, err)
	}
}

// The layer must be removable. The generic W1 reversibility proof cannot cover
// this table on its own: it snapshots sqlite_master AFTER a first drop, so a
// table that DropWikiSchema forgets would be present in both halves of the
// comparison and the assertion would pass vacuously. Hence the direct check.
func TestPageVersions_IsDroppedWithTheWikiLayer(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pv-drop.db")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer func() { _ = database.Close() }()

	proj := createTestProject(t, database, "pvdemo")
	if _, err := CreateWikiPage(database, WikiKindConcept, "pv-drop-1", "P", proj, "body"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := EnsurePageVersions(database); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	var n int
	if err := database.QueryRow("SELECT COUNT(*) FROM page_versions").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n == 0 {
		t.Fatal("precondition: the snapshot table should hold the backfilled baseline")
	}

	if err := DropWikiSchema(database); err != nil {
		t.Fatalf("drop: %v", err)
	}
	var objects int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE name LIKE 'page_versions%' OR name = 'idx_page_versions_page'`,
	).Scan(&objects); err != nil {
		t.Fatalf("count objects: %v", err)
	}
	if objects != 0 {
		var names []string
		rows, err := database.Query(
			`SELECT name FROM sqlite_master WHERE name LIKE 'page_versions%' OR name = 'idx_page_versions_page'`)
		if err == nil {
			for rows.Next() {
				var nm string
				if err := rows.Scan(&nm); err == nil {
					names = append(names, nm)
				}
			}
			_ = rows.Err()
			rows.Close()
		}
		t.Fatalf("DropWikiSchema must remove the whole snapshot layer, %d object(s) left: %v", objects, names)
	}

	// And it comes back on the next open, which is what makes the drop safe rather
	// than destructive-but-unrecoverable.
	reopened, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if err := reopened.QueryRow("SELECT COUNT(*) FROM page_versions").Scan(&n); err != nil {
		t.Fatalf("the table must be recreated on open: %v", err)
	}
}

// pageVersionObjects is the drop list; a trigger added to the DDL without a
// matching entry here would be invisible to every other test in this file.
func TestPageVersions_DropListCoversEveryObject(t *testing.T) {
	database := setupTestDB(t)
	if err := EnsurePageVersions(database); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	rows, err := database.Query(
		`SELECT type, name FROM sqlite_master WHERE name LIKE 'page_versions%' OR name = 'idx_page_versions_page'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	live := map[string]bool{}
	for rows.Next() {
		var typ, name string
		if err := rows.Scan(&typ, &name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		live[strings.ToUpper(typ)+"|"+name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(live) == 0 {
		t.Fatal("precondition: no page_versions objects found, the pattern is wrong")
	}
	declared := map[string]bool{}
	for _, o := range pageVersionObjects {
		key := o.kind + "|" + o.name
		if !live[key] {
			t.Fatalf("pageVersionObjects lists %s, which sqlite_master does not have", key)
		}
		declared[key] = true
	}
	for key := range live {
		if !declared[key] {
			t.Fatalf("object %s exists but is not in pageVersionObjects — DropWikiSchema would leave it behind", key)
		}
	}
}

// GetPageVersion / CountPageVersions, the read surface the review page uses.
func TestPageVersions_ReadAPIs(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "readapi")
	page, err := CreateWikiPage(database, WikiKindConcept, "ra-1", "P1", proj, "one")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := EnsurePageVersions(database); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := UpdateWikiPage(database, page.ID, "P1", WikiKindConcept, "two"); err != nil {
		t.Fatalf("update: %v", err)
	}
	vers, err := ListPageVersions(database, page.ID)
	if err != nil || len(vers) != 2 {
		t.Fatalf("expected two snapshots, got %d (%v)", len(vers), err)
	}

	v, err := GetPageVersion(database, vers[1].ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if v.Content != "one" || v.PageID != page.ID {
		t.Fatalf("get returned the wrong row: %+v", v)
	}
	if _, err := GetPageVersion(database, 999999); err != sql.ErrNoRows {
		t.Fatalf("absent version must be sql.ErrNoRows, got %v", err)
	}
	n, err := CountPageVersions(database)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n < 2 {
		t.Fatalf("CountPageVersions must see both rows, got %d", n)
	}
}

// DiffPageVersion is the read path the review page renders. It reuses
// internal/diff (the same line diff as notes), so what is asserted here is the
// wrapper: version-vs-current direction, and the two refusals that stop a
// reviewer reading a confident diff of the wrong documents.
func TestDiffPageVersion(t *testing.T) {
	database := setupTestDB(t)
	proj := createTestProject(t, database, "diff")
	page, err := CreateWikiPage(database, WikiKindConcept, "dv-1", "P", proj, "line one\nline two")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := EnsurePageVersions(database); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	first, err := ListPageVersions(database, page.ID)
	if err != nil || len(first) != 1 {
		t.Fatalf("expected one baseline snapshot, got %d (%v)", len(first), err)
	}

	// Identical content must diff to nothing, not to a whole-document rewrite.
	same, err := DiffPageVersion(database, page.ID, first[0].ID)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if strings.Contains(same, "+") || strings.Contains(same, "-") {
		t.Fatalf("an unchanged page should diff empty, got %q", same)
	}

	if err := UpdateWikiPage(database, page.ID, "P", WikiKindConcept, "line one\nline TWO changed"); err != nil {
		t.Fatalf("update: %v", err)
	}
	vers, err := ListPageVersions(database, page.ID)
	if err != nil || len(vers) != 2 {
		t.Fatalf("expected two snapshots, got %d (%v)", len(vers), err)
	}
	older := vers[1]
	d, err := DiffPageVersion(database, page.ID, older.ID)
	if err != nil {
		t.Fatalf("diff after edit: %v", err)
	}
	if !strings.Contains(d, "-line two") || !strings.Contains(d, "+line TWO changed") {
		t.Fatalf("diff must read version -> current, got %q", d)
	}

	// A version belonging to a DIFFERENT page must not be answerable, and a page
	// that has since been deleted must refuse rather than claim "all removed".
	other := mustCreateOtherPage(t, database, proj)
	otherVers, err := ListPageVersions(database, other.ID)
	if err != nil || len(otherVers) != 0 {
		t.Fatalf("the other page should have no snapshot yet, got %d (%v)", len(otherVers), err)
	}
	if _, err := DiffPageVersion(database, other.ID, older.ID); err == nil {
		t.Fatal("diffing another page's version must fail rather than render a bogus diff")
	}
}

func mustCreateOtherPage(t *testing.T, database *sql.DB, projectID int64) *WikiPage {
	t.Helper()
	p, err := CreateWikiPage(database, WikiKindConcept, "dv-other", "Other", projectID, "other content")
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	return p
}
