package db

import (
	"path/filepath"
	"testing"
)

// v13 repairs two classes of rows shipped by earlier versions:
//  1. note tags stored verbatim ("cbipay，样例测试,  支付 ,cbipay" as ONE tag),
//     which the frontend filter could never match after splitting on commas;
//  2. repo_meta rows for multi-repo projects, whose activity/contributors
//     were mined from the non-git container directory and are always zero.
//
// Single-repo projects mined their actual repository and must keep their
// cache untouched.
func TestInitDB_V13NormalizesTagsAndDropsMultiRepoMeta(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db1, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB (fresh): %v", err)
	}

	pidMulti := createTestProject(t, db1, "v13-multi")
	pidSingle := createTestProject(t, db1, "v13-single")
	insertRepo := func(t *testing.T, path string, pid int64) int64 {
		t.Helper()
		res, err := db1.Exec("INSERT INTO repositories (path, project_id) VALUES (?, ?)", path, pid)
		if err != nil {
			t.Fatalf("insert repo %s: %v", path, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	r1 := insertRepo(t, "/tmp/v13-multi-a", pidMulti)
	r2 := insertRepo(t, "/tmp/v13-multi-b", pidMulti)
	r3 := insertRepo(t, "/tmp/v13-single-a", pidSingle)

	for _, rid := range []int64{r1, r2, r3} {
		if err := UpsertRepoMeta(db1, rid, `["Java (Maven)"]`, "readme", `{}`, `[]`, `[]`, `{"total_commits":0,"active_days":0,"last_commit_date":"","commit_rate_30d":0,"active_months":0}`); err != nil {
			t.Fatalf("UpsertRepoMeta: %v", err)
		}
	}

	if _, err := CreateNoteEx(db1, pidMulti, "note", "body", "cbipay，样例测试,  支付 ,cbipay", "knowledge", "manual"); err != nil {
		t.Fatalf("CreateNoteEx: %v", err)
	}

	// Roll the recorded version back to 12 so the next open re-runs v13
	// against data written under the old, unnormalized regime.
	if err := writeSchemaVersion(db1, 12); err != nil {
		t.Fatalf("writeSchemaVersion(12): %v", err)
	}
	db1.Close()

	db2, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB (reopen): %v", err)
	}
	defer db2.Close()

	notes, err := ListNotes(db2, pidMulti)
	if err != nil || len(notes) != 1 {
		t.Fatalf("ListNotes: %v (%d notes)", err, len(notes))
	}
	if want := "cbipay, 样例测试, 支付"; notes[0].Tags != want {
		t.Errorf("migration left tags = %q, want %q", notes[0].Tags, want)
	}

	var n int
	for _, rid := range []int64{r1, r2} {
		if err := db2.QueryRow("SELECT COUNT(*) FROM repo_meta WHERE repository_id = ?", rid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("multi-repo repo_meta (repo %d) should have been dropped, %d row remains", rid, n)
		}
	}
	if err := db2.QueryRow("SELECT COUNT(*) FROM repo_meta WHERE repository_id = ?", r3).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("single-repo repo_meta (repo %d) must be kept, got %d rows", r3, n)
	}
}
