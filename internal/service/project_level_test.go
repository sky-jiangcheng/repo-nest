package service

import (
	"testing"

	"repo-nest/internal/db"
)

// Regression guard for a bug where MergeProjectUp / SplitProjectDown computed
// the new level and returned it, but never wrote it back — the UPDATE only
// touched is_auto_grouped / root_path / name. The project detail page renders
// level_override as "分组层级", so the number never moved no matter how many
// times the user clicked, and every later call recomputed from the same stale
// column.
func TestUpdateProjectLevel_PersistsLevelOverride(t *testing.T) {
	svc, _ := setupService(t)
	root := "/tmp/leveltest"
	pid := seedProject(t, svc.db, "grp", root)
	seedRepo(t, svc.db, root+"/a", pid)
	seedRepo(t, svc.db, root+"/b", pid)

	assertLevel := func(step string, want int) {
		t.Helper()
		p, err := db.GetProjectByID(svc.db, pid)
		if err != nil {
			t.Fatalf("%s: GetProjectByID: %v", step, err)
		}
		if p.LevelOverride != want {
			t.Errorf("%s: stored level_override = %d, want %d", step, p.LevelOverride, want)
		}
	}

	// Split creates per-repo projects and drops the group to -1.
	if _, err := svc.UpdateProjectLevel(pid, "down"); err != nil {
		t.Fatalf("down: %v", err)
	}
	assertLevel("after down", -1)

	// Merging back must climb out of -1, not stay there reporting +1 forever.
	if _, err := svc.UpdateProjectLevel(pid, "up"); err != nil {
		t.Fatalf("up: %v", err)
	}
	assertLevel("after up", 0)
}

// Two consecutive "up" calls must each advance the stored level. Before the
// fix both returned 1 while the second was a no-op.
func TestUpdateProjectLevel_ConsecutiveUpEachAdvance(t *testing.T) {
	svc, _ := setupService(t)
	dir := "/tmp/leveltest2"
	pid := seedProject(t, svc.db, "only", dir)

	var last int
	for i := 1; i <= 3; i++ {
		res, err := svc.UpdateProjectLevel(pid, "up")
		if err != nil {
			t.Fatalf("up #%d: %v", i, err)
		}
		p, err := db.GetProjectByID(svc.db, pid)
		if err != nil {
			t.Fatalf("up #%d: GetProjectByID: %v", i, err)
		}
		if res.NewLevel != i {
			t.Errorf("up #%d: returned new_level = %d, want %d", i, res.NewLevel, i)
		}
		if p.LevelOverride != i {
			t.Errorf("up #%d: stored level_override = %d, want %d", i, p.LevelOverride, i)
		}
		last = i
	}
	if last != 3 {
		t.Fatalf("expected to reach level 3, got %d", last)
	}
}

// Splitting a single-repo project has nothing to split off, but the level must
// still move — otherwise the "-" button looks dead on leaf projects.
func TestUpdateProjectLevel_DownOnSingleRepoStillMovesLevel(t *testing.T) {
	svc, _ := setupService(t)
	dir := "/tmp/leveltest3"
	pid := seedProject(t, svc.db, "leaf", dir)
	seedRepo(t, svc.db, dir+"/only", pid)

	res, err := svc.UpdateProjectLevel(pid, "down")
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	p, err := db.GetProjectByID(svc.db, pid)
	if err != nil {
		t.Fatalf("GetProjectByID: %v", err)
	}
	if res.NewLevel != -1 || p.LevelOverride != -1 {
		t.Errorf("down on single-repo project: returned %d, stored %d; want -1/-1", res.NewLevel, p.LevelOverride)
	}
}

// A split must be reversible. The project a split came out of still occupies
// the exact path the merge tries to move back into, and projects.root_path is
// UNIQUE — so the sibling scan (which only compared filepath.Dir) missed it and
// every merge failed with "UNIQUE constraint failed: projects.root_path",
// rolling the whole thing back. The user-visible effect was a one-way door:
// split once, never regroup again.
func TestUpdateProjectLevel_SplitThenMergeIsReversible(t *testing.T) {
	svc, _ := setupService(t)
	root := "/tmp/leveltest4"
	group := seedProject(t, svc.db, "grp", root)
	seedRepo(t, svc.db, root+"/a", group)
	seedRepo(t, svc.db, root+"/b", group)
	seedRepo(t, svc.db, root+"/c", group)

	if _, err := svc.UpdateProjectLevel(group, "down"); err != nil {
		t.Fatalf("down: %v", err)
	}
	// Collect the ids that still exist: the group plus whatever the split
	// created (the survivors keep their notes, so the group id is still live).
	var live []int64
	for id := int64(1); id <= 8; id++ {
		if p, err := db.GetProjectByID(svc.db, id); err == nil && p != nil {
			live = append(live, p.ID)
		}
	}
	if len(live) != 3 {
		t.Fatalf("after split want 3 projects, got %d (ids %v)", len(live), live)
	}

	// Merge any of the split-off projects back up. This is the step that used
	// to abort on the root_path unique constraint.
	if _, err := svc.UpdateProjectLevel(live[0], "up"); err != nil {
		t.Fatalf("up after split: %v", err)
	}

	var after []int64
	for id := int64(1); id <= 8; id++ {
		if p, err := db.GetProjectByID(svc.db, id); err == nil && p != nil {
			after = append(after, p.ID)
		}
	}
	if len(after) != 1 {
		t.Fatalf("after merge want 1 project, got %d (ids %v)", len(after), after)
	}
	p, err := db.GetProjectByID(svc.db, after[0])
	if err != nil {
		t.Fatalf("GetProjectByID: %v", err)
	}
	// Back at the original root, holding all three repos.
	if p.RootPath != root {
		t.Errorf("merged root_path = %q, want %q", p.RootPath, root)
	}
	repos, err := db.GetRepositoriesByProjectID(svc.db, p.ID)
	if err != nil {
		t.Fatalf("GetRepositoriesByProjectID: %v", err)
	}
	if len(repos) != 3 {
		t.Errorf("merged project holds %d repos, want 3", len(repos))
	}
}