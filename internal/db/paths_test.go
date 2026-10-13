package db

import (
	"testing"
)

// Path matcher tests (internal/db is where the rules live; see paths.go).

func TestIsPathAncestorIsSegmentAware(t *testing.T) {
	cases := []struct {
		parent, child string
		want          bool
		why           string
	}{
		{"/a/b", "/a/b/c", true, "ordinary ancestor"},
		{"/a/b", "/a/bc", false, "string prefix is NOT a directory ancestor — the classic bug"},
		{"/a/b", "/a/b", false, "a path is not its own ancestor (that is the exact tier)"},
		{"/a/b", "/a", false, "the relation does not run upwards"},
		{"", "/a", false, "empty parent matches nothing"},
		{"/a", "", false, "empty child matches nothing"},
		{"/", "/a", false, "root is normalised away by NormalizeMatchPath, so it must not adopt everything"},
	}
	for _, tc := range cases {
		if got := isPathAncestor(tc.parent, tc.child); got != tc.want {
			t.Errorf("isPathAncestor(%q, %q) = %v, want %v (%s)", tc.parent, tc.child, got, tc.want, tc.why)
		}
	}
}

func TestNormalizeMatchPath(t *testing.T) {
	for in, want := range map[string]string{
		"/a/b/":           "/a/b",
		"/a/b":            "/a/b",
		"/a//b":           "/a/b",
		"/a/b/../c":       "/a/c",
		"   /a/b   ":      "/a/b",
		"":                "",
		"/":               "/",
		"relative/subdir": "relative/subdir",
	} {
		if got := NormalizeMatchPath(in); got != want {
			t.Errorf("NormalizeMatchPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// PickRepositoryForPath / PickProjectForPath as pure functions, so the tier rules
// are testable without a database.
func TestPickRepositoryForPath(t *testing.T) {
	repos := []Repository{
		{ID: 1, Path: "/p/CBiPay/pay-common"},
		{ID: 2, Path: "/p/CBiPay/pay-account"},
		{ID: 3, Path: "/p/CBiPay/pay-common/vendor/lib"},
	}
	cases := []struct {
		dir     string
		wantID  int64
		wantHow string
	}{
		{"/p/CBiPay/pay-common", 1, PathMatchExact},
		{"/p/CBiPay/pay-common/src/main", 1, PathMatchAncestor},
		// deepest wins inside the outer repository
		{"/p/CBiPay/pay-common/vendor/lib/x", 3, PathMatchAncestor},
		// a sibling that merely shares a string prefix must not match
		{"/p/CBiPay/pay-commonx", 0, ""},
		{"/p/CBiPay", 0, ""},
		{"/p/CBiPay/PAY-COMMON", 1, PathMatchCaseFolded},
	}
	for _, tc := range cases {
		got, how, err := PickRepositoryForPath(repos, tc.dir)
		if err != nil {
			t.Fatalf("%q: %v", tc.dir, err)
		}
		if tc.wantID == 0 {
			if got != nil {
				t.Errorf("pickRepository(%q) = %+v, want no match", tc.dir, got)
			}
			continue
		}
		if got == nil || got.ID != tc.wantID {
			t.Errorf("pickRepository(%q) = %+v, want id %d", tc.dir, got, tc.wantID)
			continue
		}
		if how != tc.wantHow {
			t.Errorf("pickRepository(%q) how = %q, want %q", tc.dir, how, tc.wantHow)
		}
	}
}

// The container restriction, expressed on the pure function: an auto-grouped
// project may match exactly but never by ancestry.
func TestPickProjectForPathContainerRule(t *testing.T) {
	projects := []Project{
		{ID: 1, Name: "home", RootPath: "/Users/dev", IsAutoGrouped: true},
		{ID: 25, Name: "CBiPay", RootPath: "/p/CBiPay"},
	}
	// Ancestor under the container only: refused.
	got, how, err := PickProjectForPath(projects, "/Users/dev/Downloads/scratch", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("the catch-all must not adopt a subdirectory, got %s (%s)", got.Name, how)
	}
	// Same query with the restriction lifted: it does (this is what the flag is for).
	got, how, err = PickProjectForPath(projects, "/Users/dev/Downloads/scratch", true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 1 {
		t.Fatalf("allowCatchAll should let the container answer, got %+v (%s)", got, how)
	}
	// Exact match on the container is allowed even with the restriction on.
	got, how, err = PickProjectForPath(projects, "/Users/dev", false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 1 || how != PathMatchExact {
		t.Fatalf("an exact match on a container should resolve, got %+v (%s)", got, how)
	}
	// A collected project wins by ancestry as normal.
	got, how, err = PickProjectForPath(projects, "/p/CBiPay/docs", false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 25 || how != PathMatchAncestor {
		t.Fatalf("collected project should adopt its subdirectory, got %+v (%s)", got, how)
	}
}
