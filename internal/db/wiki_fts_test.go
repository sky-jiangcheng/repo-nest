package db

import (
	"testing"
)

// SearchWikiPages is the retrieval half of graph-aware evidence: whatever it
// returns becomes the seed set, so a query it cannot serve starves the walk
// entirely. The notes path already handles the two shapes the trigram index
// cannot (short CJK terms, and zero-row strict AND); the page path must not be
// the one place a real question silently returns nothing.

// A 2-character CJK term is below the trigram tokenizer's floor, so FTS cannot
// match it — but a page whose body contains it is obviously the answer. The
// LIKE scan covers exactly this, the same way the notes path does.
func TestSearchWikiPages_ShortCJKTermFallsBackToLike(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "short-cjk")
	page, err := CreateWikiPage(db, WikiKindEntity, "alpha-hub", "Alpha Hub", pid,
		"alpha 重试与路由的规则正文")
	if err != nil {
		t.Fatal(err)
	}

	got, err := SearchWikiPages(db, "重试", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != page.ID {
		t.Errorf("SearchWikiPages(%q) = %+v, want the page whose body contains it", "重试", got)
	}
}

// Strict AND across two terms in DIFFERENT pages yields nothing, which is the
// single most common real-query shape ("迁移 手册" when one page says 迁移 and
// another says 手册). Relaxing to OR only ever fires on an empty strict result,
// so it cannot degrade a currently-good result set — the M3-C constraint.
func TestSearchWikiPages_RelaxesToORWhenStrictANDIsEmpty(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "or-relax")
	a, err := CreateWikiPage(db, WikiKindEntity, "alpha-hub", "Alpha Hub", pid,
		"迁移按版本顺序执行，写入 schema_version")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateWikiPage(db, WikiKindEntity, "beta-hub", "Beta Hub", pid,
		"手册说明副本演练与回滚预案")
	if err != nil {
		t.Fatal(err)
	}

	got, err := SearchWikiPages(db, "迁移 手册", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("SearchWikiPages(%q) = %+v, want both pages (relaxed OR)", "迁移 手册", got)
	}
	seen := map[int64]bool{}
	for _, p := range got {
		seen[p.ID] = true
	}
	if !seen[a.ID] || !seen[b.ID] {
		t.Errorf("relaxed result = %v, want both %d and %d", seen, a.ID, b.ID)
	}
}

// The relaxation must stay subordinate: when a page satisfies BOTH terms, the
// strict result stands and a one-term page must not sneak in. This is the
// difference between "search got better" and "search got noisier".
func TestSearchWikiPages_StrictANDBeatsRelaxation(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "strict-and")
	both, err := CreateWikiPage(db, WikiKindEntity, "alpha-hub", "Alpha Hub", pid,
		"迁移手册：版本顺序、副本演练、回滚预案")
	if err != nil {
		t.Fatal(err)
	}
	oneTerm, err := CreateWikiPage(db, WikiKindEntity, "beta-hub", "Beta Hub", pid,
		"只提到迁移这个词的页面")
	if err != nil {
		t.Fatal(err)
	}

	got, err := SearchWikiPages(db, "迁移 手册", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != both.ID {
		t.Errorf("SearchWikiPages(%q) = %+v, want only the page matching both terms (%d, not %d)",
			"迁移 手册", got, both.ID, oneTerm.ID)
	}
}

// The LIKE fallback inherits the approved-only gate: a pending compiler page
// that matches must not become evidence.
func TestSearchWikiPages_FallbacksStayApprovedOnly(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "approved-only")
	if _, err := CreateWikiPage(db, WikiKindEntity, "alpha-hub", "Alpha Hub", pid,
		"重试与退避的规则正文"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateWikiPageAs(db, WikiKindEntity, "beta-draft", "Beta Draft", pid,
		"重试草稿正文", WikiStatusPending, "wiki-compile"); err != nil {
		t.Fatal(err)
	}

	got, err := SearchWikiPages(db, "重试", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("SearchWikiPages(%q) = %+v, want only the approved page", "重试", got)
	}
}
