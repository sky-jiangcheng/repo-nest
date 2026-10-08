package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"repo-nest/internal/db"
)

// buildExportFixture seeds pages across kinds, links between them, and one note
// attached to a page, and returns the project id.
func buildExportFixture(t *testing.T, svc *Service) (projectID int64, pageIDs map[string]int64) {
	t.Helper()
	projectID = seedProject(t, svc.db, "export", "/tmp/export")
	pageIDs = map[string]int64{}

	mk := func(kind, slug, title, body string) int64 {
		t.Helper()
		p, err := db.CreateWikiPage(svc.db, kind, slug, title, projectID, body)
		if err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
		pageIDs[slug] = p.ID
		return p.ID
	}
	gw := mk(db.WikiKindEntity, "payment-gateway", "Payment Gateway", "Handles retries and idempotency keys.")
	timeout := mk(db.WikiKindConcept, "session-timeout", "Session Timeout", "Idle expiry and refresh-token rotation.")
	src := mk(db.WikiKindSource, "incident-2026-07", "Incident 2026-07", "Postmortem of the stuck retry storm.")
	syn := mk(db.WikiKindSynthesis, "reliability", "Reliability", "How the pieces fit.")
	// Body text deliberately links at a page that does not exist: this is the
	// rot path the constraints cannot cover, and the export must notice it.
	mk(db.WikiKindQuery, "why-retries", "Why retries?", "Filed answer: because the network lies. See [[no-such-page]].")

	if err := db.LinkWikiPages(svc.db, src, gw, "explains"); err != nil {
		t.Fatal(err)
	}
	if err := db.LinkWikiPages(svc.db, syn, timeout, "depends-on"); err != nil {
		t.Fatal(err)
	}
	// Deleting a linked page must take the edge with it: page->page links cannot
	// dangle, and the export relies on that (no "missing target" branch to test).
	ghost := mk(db.WikiKindEntity, "ghost", "Ghost", "will be deleted")
	if err := db.LinkWikiPages(svc.db, gw, ghost, "mentions"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteWikiPage(svc.db, ghost); err != nil {
		t.Fatal(err)
	}
	if edges, err := db.WikiEdgesFrom(svc.db, gw); err != nil {
		t.Fatal(err)
	} else {
		for _, e := range edges {
			if e.Slug == "ghost" {
				t.Fatal("deleting a page left a dangling edge behind")
			}
		}
	}

	n, err := db.CreateNoteEx(svc.db, projectID, "Retry storm notes", "body of the note", "", "knowledge", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AttachNoteToPage(svc.db, n.ID, pageIDs["incident-2026-07"]); err != nil {
		t.Fatal(err)
	}
	return projectID, pageIDs
}

func TestExportWikiTree_WritesReadableTree(t *testing.T) {
	svc, _ := setupService(t)
	_, pages := buildExportFixture(t, svc)
	out := filepath.Join(t.TempDir(), "vault")

	report, err := svc.ExportWikiTree(out, 0, false)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if report.Pages != 5 {
		t.Errorf("report.Pages = %d, want 5 (the ghost page was deleted)", report.Pages)
	}

	// Every kind directory exists with its page in it.
	for slug, dir := range map[string]string{
		"payment-gateway":  "entities",
		"session-timeout":  "concepts",
		"incident-2026-07": "sources",
		"reliability":      "synthesis",
		"why-retries":      "queries",
	} {
		p := filepath.Join(out, "wiki", dir, slug+".md")
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
		body := string(raw)
		if !strings.HasPrefix(body, "---\n") || !strings.Contains(body, "source: reponest-export") {
			t.Errorf("%s lacks the export frontmatter", p)
		}
		if !strings.Contains(body, fmt.Sprintf("reponest_page_id: %d", pages[slug])) {
			t.Errorf("%s does not carry its page id", p)
		}
	}

	// Wikilinks must resolve to files that exist in the same export — an index or
	// page pointing at a missing note is precisely what makes a graph view lie.
	wikilink := regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)
	var targets []string
	err = filepath.WalkDir(filepath.Join(out, "wiki"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, m := range wikilink.FindAllStringSubmatch(string(raw), -1) {
			targets = append(targets, m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) == 0 {
		t.Fatal("no wikilinks anywhere; the graph would be empty and the test vacuous")
	}
	for _, slug := range targets {
		var found bool
		for _, dir := range []string{"entities", "concepts", "sources", "synthesis", "queries"} {
			if _, err := os.Stat(filepath.Join(out, "wiki", dir, slug+".md")); err == nil {
				found = true
			}
		}
		// The fixture plants exactly one broken body link to exercise the counter.
		// Anything else that fails to resolve is a real defect.
		if !found && slug != "no-such-page" {
			t.Errorf("wikilink [[%s]] has no exported file", slug)
		}
	}
	if report.DanglingLinks == 0 {
		t.Error("the planted [[no-such-page]] link was not counted as dangling")
	}

	// Notes behind a page are named, so "where did this come from" survives export.
	srcFile, err := os.ReadFile(filepath.Join(out, "wiki", "sources", "incident-2026-07.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(srcFile), "## 来源笔记") || !strings.Contains(string(srcFile), "Retry storm notes") {
		t.Error("source notes missing from the exported page")
	}

	// Backlinks appear on the target of an edge.
	back, err := os.ReadFile(filepath.Join(out, "wiki", "entities", "payment-gateway.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(back), "## 反向链接") || !strings.Contains(string(back), "incident-2026-07") {
		t.Error("backlink section missing")
	}

	// Index lists every page.
	idx, err := os.ReadFile(filepath.Join(out, "wiki", "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	for slug := range pages {
		if slug == "ghost" {
			continue
		}
		if !strings.Contains(string(idx), "[["+slug+"|") {
			t.Errorf("index missing %s", slug)
		}
	}

	// Deleting a page and re-exporting into the SAME directory must surface its
	// orphaned file as stale — and leave it on disk. Pruning would mean the export
	// owns the tree, which is precisely what ADR-0014 决策 2 refuses to assume,
	// and it cannot tell our leftovers from the user's own notes anyway.
	same := filepath.Join(t.TempDir(), "same")
	if _, err := svc.ExportWikiTree(same, 0, false); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(same, "wiki", "concepts", "session-timeout.md")
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("expected the concept page exported first: %v", err)
	}
	if err := db.DeleteWikiPage(svc.db, pages["session-timeout"]); err != nil {
		t.Fatal(err)
	}
	reexport, err := svc.ExportWikiTree(same, 0, true)
	if err != nil {
		t.Fatalf("re-export into the same directory must be allowed: %v", err)
	}
	var foundStale bool
	for _, f := range reexport.StaleFiles {
		if f == "wiki/concepts/session-timeout.md" {
			foundStale = true
		}
	}
	if !foundStale {
		t.Errorf("StaleFiles = %v, want the deleted page's file listed", reexport.StaleFiles)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Error("the export deleted a file; it must only report stale ones")
	}
	// Re-exporting without --force is allowed here on purpose: the guard blocks
	// foreign files (see TestExportWikiTree_RefusesNonEmptyTarget), while our own
	// wiki subtree is exactly what an idempotent re-export must be able to rewrite.
	if _, err := svc.ExportWikiTree(same, 0, false); err != nil {
		t.Errorf("re-export into our own subtree must not need --force: %v", err)
	}

	// Manifest completeness: it must name every written file and nothing else, so
	// a half-finished export is detectable (same role as the packaging sha256 gate).
	var manifest WikiExportReport
	raw, err := os.ReadFile(filepath.Join(out, "wiki", "EXPORT-MANIFEST.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	onDisk := map[string]bool{}
	_ = filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(out, path)
		if rerr == nil {
			onDisk[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	named := map[string]bool{}
	for _, f := range manifest.Files {
		named[f.Path] = true
		if !onDisk[f.Path] {
			t.Errorf("manifest lists %s but it is not on disk", f.Path)
		}
	}
	for rel := range onDisk {
		if rel == "wiki/EXPORT-MANIFEST.json" {
			continue // written after the manifest body, by definition
		}
		if !named[rel] {
			t.Errorf("%s was written but is not in the manifest", rel)
		}
	}
}

// Determinism: re-export must produce identical bytes, or "did anything change"
// becomes unanswerable and Obsidian's own diff noise buries real edits.
func TestExportWikiTree_IsDeterministic(t *testing.T) {
	svc, _ := setupService(t)
	_, _ = buildExportFixture(t, svc)
	dir1, dir2 := filepath.Join(t.TempDir(), "v"), filepath.Join(t.TempDir(), "v")

	r1, err := svc.ExportWikiTree(dir1, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := svc.ExportWikiTree(dir2, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	sum := func(r *WikiExportReport) map[string]string {
		out := map[string]string{}
		for _, f := range r.Files {
			out[f.Path] = f.SHA256
		}
		return out
	}
	a, b := sum(r1), sum(r2)
	if len(a) != len(b) {
		t.Fatalf("file sets differ: %d vs %d", len(a), len(b))
	}
	for k, v := range a {
		if b[k] == v {
			continue
		}
		// Name the first differing byte pair rather than just "digest differs":
		// with a manifest in the set, the useful question is always what inside it
		// moved, and guessing that from a hash is a waste of the next reader's time.
		ra, _ := os.ReadFile(filepath.Join(dir1, filepath.FromSlash(k)))
		rb, _ := os.ReadFile(filepath.Join(dir2, filepath.FromSlash(k)))
		line := firstDiffLine(string(ra), string(rb))
		t.Fatalf("%s differs between runs\n  A: %s\n  B: %s", k, line, "")
	}
}

// firstDiffLine returns the first line of a that differs from b (or a marker when
// one side ran out).
func firstDiffLine(a, b string) string {
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(la); i++ {
		if i >= len(lb) {
			return "A has extra line " + la[i]
		}
		if la[i] != lb[i] {
			return fmt.Sprintf("line %d: A=%q B=%q", i+1, la[i], lb[i])
		}
	}
	return "identical content, different digest?"
}

func TestExportWikiTree_RefusesNonEmptyTarget(t *testing.T) {
	svc, _ := setupService(t)
	_, _ = buildExportFixture(t, svc)
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "someone-elses-notes.md"), []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ExportWikiTree(out, 0, false); !errors.Is(err, ErrWikiExportNotEmpty) {
		t.Fatalf("err = %v, want ErrWikiExportNotEmpty", err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "someone-elses-notes.md"))
	if err != nil || string(raw) != "keep me" {
		t.Error("the refused export still touched the user's file")
	}
	if _, err := os.Stat(filepath.Join(out, "wiki")); !os.IsNotExist(err) {
		t.Error("a refused export must not create the wiki subtree either")
	}
}

// Slugs come from user-authored titles. The export is a filesystem writer, so
// traversal is rejected there rather than trusted to the normalizer.
func TestExportWikiTree_RejectsSlugTraversal(t *testing.T) {
	svc, _ := setupService(t)
	projectID := seedProject(t, svc.db, "slug", "/tmp/slug")
	if _, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "../escaped", "Escaped", projectID, "x"); err != nil {
		t.Fatal(err)
	}
	good, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "fine", "Fine", projectID, "y")
	if err != nil {
		t.Fatal(err)
	}
	_ = good

	out := t.TempDir()
	report, err := svc.ExportWikiTree(out, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.SkippedPages != 1 {
		t.Errorf("SkippedPages = %d, want the hostile slug reported as skipped", report.SkippedPages)
	}
	if _, err := os.Stat(filepath.Join(out, "escaped.md")); !os.IsNotExist(err) {
		t.Fatal("a hostile slug wrote outside the wiki subtree")
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "entities", "fine.md")); err != nil {
		t.Errorf("the benign page was not exported: %v", err)
	}
}

// -project scopes the export, and an empty store must still produce an index that
// says so rather than a directory nobody can interpret.
func TestExportWikiTree_ScopingAndEmpty(t *testing.T) {
	svc, _ := setupService(t)
	a, _ := buildExportFixture(t, svc)
	other := seedProject(t, svc.db, "other", "/tmp/other")
	if _, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "elsewhere", "Elsewhere", other, "not mine"); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "one")
	if _, err := svc.ExportWikiTree(out, a, false); err != nil {
		t.Fatal(err)
	}
	idx, err := os.ReadFile(filepath.Join(out, "wiki", "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(idx), "elsewhere") {
		t.Error("another project's page leaked into a scoped export")
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "concepts", "elsewhere.md")); !os.IsNotExist(err) {
		t.Error("a scoped export wrote a foreign page")
	}

	// Empty: no pages, but the index exists and says why.
	// A second service over its own in-memory DB: the empty case needs a store with
	// no pages, not a fixture project deleted afterwards (which would leave the
	// derived rows behind and prove nothing).
	fresh, _ := setupService(t)
	empty := filepath.Join(t.TempDir(), "none")
	rep, err := fresh.ExportWikiTree(empty, 0, false)
	if err != nil {
		t.Fatalf("exporting an empty wiki must still work: %v", err)
	}
	if rep.Pages != 0 {
		t.Errorf("Pages = %d, want 0", rep.Pages)
	}
	raw, err := os.ReadFile(filepath.Join(empty, "wiki", "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "没有页面") {
		t.Error("an empty export should say the layer is unpopulated")
	}
	var names []string
	_ = filepath.WalkDir(filepath.Join(empty, "wiki"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, filepath.Base(p))
		}
		return nil
	})
	sort.Strings(names)
	if len(names) != 2 || names[0] != "EXPORT-MANIFEST.json" || names[1] != "index.md" {
		t.Errorf("empty export wrote %v, want only the index and the manifest", names)
	}
}
