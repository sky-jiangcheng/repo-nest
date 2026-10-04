package grouper

import (
	"path/filepath"
	"testing"

	"repo-nest/internal/scanner"
)

func TestGroupRepositories_Empty(t *testing.T) {
	groups := GroupRepositories(nil)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for nil input, got %d", len(groups))
	}

	groups = GroupRepositories([]scanner.RepoInfo{})
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for empty input, got %d", len(groups))
	}
}

func TestGroupRepositories_SingleRepo(t *testing.T) {
	repos := []scanner.RepoInfo{
		{Path: "/home/user/projects/myapp", Depth: 1},
	}
	groups := GroupRepositories(repos)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "projects" {
		t.Errorf("expected group name 'projects', got '%s'", groups[0].Name)
	}
	if len(groups[0].Repos) != 1 {
		t.Errorf("expected 1 repo in group, got %d", len(groups[0].Repos))
	}
}

func TestGroupRepositories_MultipleReposSameParent(t *testing.T) {
	repos := []scanner.RepoInfo{
		{Path: filepath.Join("/tmp", "workspace", "frontend"), Depth: 2},
		{Path: filepath.Join("/tmp", "workspace", "backend"), Depth: 2},
	}
	groups := GroupRepositories(repos)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Repos) != 2 {
		t.Errorf("expected 2 repos in group, got %d", len(groups[0].Repos))
	}
}

func TestGroupRepositories_NestedRepos(t *testing.T) {
	repos := []scanner.RepoInfo{
		{Path: filepath.Join("/tmp", "monorepo"), Depth: 0},
		{Path: filepath.Join("/tmp", "monorepo", "sub1"), Depth: 1},
		{Path: filepath.Join("/tmp", "monorepo", "sub2"), Depth: 1},
	}
	groups := GroupRepositories(repos)
	if len(groups) < 2 {
		t.Fatalf("expected at least 2 groups for nested repos, got %d", len(groups))
	}
}

func TestGroupRepositories_DifferentParents(t *testing.T) {
	repos := []scanner.RepoInfo{
		{Path: filepath.Join("/tmp", "proj-a", "repo1"), Depth: 2},
		{Path: filepath.Join("/tmp", "proj-b", "repo2"), Depth: 2},
	}
	groups := GroupRepositories(repos)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups for different parents, got %d", len(groups))
	}
}

func TestIsDirectChild(t *testing.T) {
	tests := []struct {
		parent, child string
		expected      bool
	}{
		{"/home", "/home/user", true},
		{"/home", "/home/user/projects", false},
		{"/", "/home", true},
		{"/home", "/etc", false},
	}
	for _, tc := range tests {
		result := isDirectChild(tc.parent, tc.child)
		if result != tc.expected {
			t.Errorf("isDirectChild(%s, %s) = %v, want %v", tc.parent, tc.child, result, tc.expected)
		}
	}
}

// Overlapping roots reach the "parent is also a repo" rule: when a repo is
// discovered as its own root (depth 0) it sorts BEFORE its parent repo
// (discovered deeper from an outer root), so the child is processed first and
// the parent branch fires. Locks the grouping behaviour: parent and child
// become two separate auto-grouped projects instead of one merged group.
func TestGroupRepositories_OverlappingRootsParentAndChild(t *testing.T) {
	repos := []scanner.RepoInfo{
		// /outer walks discover the parent repo at depth 1; its own walk
		// stops at the repo boundary, so the nested child repo is only
		// discovered by scanning /outer/parent/child as a root (depth 0).
		{Path: "/w/outer/parent", Depth: 1},
		{Path: "/w/outer/parent/child", Depth: 0},
	}
	groups := GroupRepositories(repos)
	if len(groups) != 2 {
		t.Fatalf("expected the Rule-3 pair (parent + child group), got %+v", groups)
	}
	// Both groups carry the parent's RootPath — the synthesized parent group
	// and the sibling group under it. This is the documented quirk: the DB
	// layer upserts projects by root_path, so the effective outcome is one
	// project rooted at /w/outer/parent holding both repos.
	seen := map[string]bool{}
	reposSeen := map[string]bool{}
	for _, g := range groups {
		if g.RootPath != "/w/outer/parent" {
			t.Fatalf("expected every group to root at the parent repo, got %+v", g)
		}
		seen[g.RootPath] = true
		for _, r := range g.Repos {
			reposSeen[r.Path] = true
			// The synthesized parent RepoInfo carries its real discovered
			// depth, not a hardcoded 0.
			if r.Path == "/w/outer/parent" && r.Depth != 1 {
				t.Fatalf("parent repo depth should be the discovered value 1, got %d", r.Depth)
			}
		}
	}
	if !reposSeen["/w/outer/parent"] || !reposSeen["/w/outer/parent/child"] {
		t.Fatalf("both repos must survive grouping, got %v", reposSeen)
	}
	if len(seen) != 1 {
		t.Fatalf("expected a single effective root path, got %v", seen)
	}
}
