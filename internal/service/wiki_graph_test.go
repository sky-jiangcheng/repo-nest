package service

import (
	"testing"

	"repo-nest/internal/db"
)

// ADR-0018 lane 1: approving a page materializes its body [[wikilinks]] as
// `mentions` edges — deterministically, only to pages that exist, never to
// itself, and idempotently.
func TestWikiApprove_ExtractsMentionsEdges(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "kg", "/tmp/kg")

	target, err := db.CreateWikiPageAs(svc.db, db.WikiKindEntity, "alpha", "Alpha", pid,
		"target body long enough to survive the thin page check in isolation here", db.WikiStatusApproved, "manual")
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateWikiPageAs(svc.db, db.WikiKindConcept, "beta", "Beta", pid,
		"other target body long enough to survive the thin page check here too", db.WikiStatusApproved, "manual")
	if err != nil {
		t.Fatal(err)
	}

	// A pending page linking alpha, beta, a nonexistent ghost, and itself.
	body := "参见 [[alpha]]、[[beta|Beta 概念]] 与 [[ghost]]，又误链到自己 [[self-page]]。" +
		"这段正文刻意写长，以保证除链接抽取之外的检查不会干扰本测试的断言。"
	p, err := db.CreateWikiPageAs(svc.db, db.WikiKindConcept, "self-page", "Self Page", pid,
		body, db.WikiStatusPending, compileSourceTag)
	if err != nil {
		t.Fatal(err)
	}

	// Before approval a pending page has no mentions edges.
	if e := mentionsFrom(t, svc, p.ID); len(e) != 0 {
		t.Fatalf("pending page already has mentions edges: %v", e)
	}

	if err := svc.ApproveWikiPage(p.ID); err != nil {
		t.Fatal(err)
	}

	got := mentionsFrom(t, svc, p.ID)
	if len(got) != 2 {
		t.Fatalf("mentions edges after approve = %d, want 2 (alpha, beta); got %v", len(got), got)
	}
	for _, e := range got {
		if e.PageID != target.ID && e.PageID != other.ID {
			t.Errorf("unexpected mention target %d", e.PageID)
		}
	}

	// Idempotent: re-extracting adds nothing (UNIQUE(from,to,relation) collapses).
	loaded, err := db.GetWikiPageByID(svc.db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	svc.extractMentions(loaded)
	if again := mentionsFrom(t, svc, p.ID); len(again) != 2 {
		t.Errorf("re-extraction changed edge count to %d, want still 2", len(again))
	}
}

// The structural relation checks — supersedes inversion, a part-of cycle, and a
// one-sided contradicts — all fire, and lint still only writes todos (it never
// touches page content; that invariant is guarded by
// TestWikiLint_NeverMutatesPages, this test asserts the findings exist).
func TestWikiLint_RelationShape(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "lintrel", "/tmp/lintrel")

	pa := mustApprovedPage(t, svc, pid, "pa")
	pb := mustApprovedPage(t, svc, pid, "pb")
	pc := mustApprovedPage(t, svc, pid, "pc")

	// pa claims to supersede pb, but pb is newer → inversion.
	if err := db.LinkWikiPages(svc.db, pa.ID, pb.ID, db.RelationSupersedes); err != nil {
		t.Fatal(err)
	}
	setUpdatedAt(t, svc, pa.ID, "2020-01-01 00:00:00")
	setUpdatedAt(t, svc, pb.ID, "2030-01-01 00:00:00")

	// part-of cycle: pa ⊂ pb and pb ⊂ pa.
	if err := db.LinkWikiPages(svc.db, pa.ID, pb.ID, db.RelationPartOf); err != nil {
		t.Fatal(err)
	}
	if err := db.LinkWikiPages(svc.db, pb.ID, pa.ID, db.RelationPartOf); err != nil {
		t.Fatal(err)
	}

	// one-sided contradiction: pc conflicts pa with no reverse edge.
	if err := db.LinkWikiPages(svc.db, pc.ID, pa.ID, db.RelationContradicts); err != nil {
		t.Fatal(err)
	}

	rep, err := svc.RunWikiLint(pid, false)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	var seen []string
	for _, f := range rep.Findings {
		have[f.Check] = true
		seen = append(seen, f.Check)
	}
	for _, want := range []string{
		"relation-supersedes-inverted",
		"relation-cycle",
		"relation-contradicts-one-sided",
	} {
		if !have[want] {
			t.Errorf("missing relation finding %q; checks present: %v", want, seen)
		}
	}
}

func mentionsFrom(t *testing.T, svc *Service, pageID int64) []db.PageEdge {
	t.Helper()
	edges, err := db.WikiEdgesFrom(svc.db, pageID)
	if err != nil {
		t.Fatal(err)
	}
	var out []db.PageEdge
	for _, e := range edges {
		if e.Relation == db.RelationMentions {
			out = append(out, e)
		}
	}
	return out
}

func mustApprovedPage(t *testing.T, svc *Service, pid int64, slug string) *db.WikiPage {
	t.Helper()
	p, err := db.CreateWikiPageAs(svc.db, db.WikiKindEntity, slug, "T "+slug, pid,
		"body padding to clear the thin page threshold so this test asserts "+
			"only on the relation shape checks and not on length noise", db.WikiStatusApproved, "manual")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func setUpdatedAt(t *testing.T, svc *Service, id int64, ts string) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE wiki_pages SET updated_at = ? WHERE id = ?`, ts, id); err != nil {
		t.Fatal(err)
	}
}
