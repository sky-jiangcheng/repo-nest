// Command .e2e-seed builds a realistic pre-v21 dashboard.db: a "repo-nest"
// knowledge base with notes, approved wiki pages, a pending compiler page, and
// page_links carrying the LEGACY free-text relation values a real user's v20
// database would have (compiled-from / cites / conflicts-with / model coinages,
// including a duplicate pair that both fold onto ref). It then stamps
// schema_version=20 so the v21 migration is genuinely pending — exactly the
// state the wiki-rehearse harness exists to rehearse.
package main

import (
	"fmt"
	"os"

	"repo-nest/internal/db"
)

func main() {
	path := "/tmp/e2e/dashboard.db"
	_ = os.Remove(path)
	d, err := db.InitDB(path)
	if err != nil {
		fmt.Println("init:", err)
		os.Exit(1)
	}
	defer d.Close()

	if _, err := d.Exec(`INSERT INTO projects (name, root_path) VALUES ('repo-nest', '/repos/repo-nest')`); err != nil {
		fmt.Println("project:", err)
		os.Exit(1)
	}
	var pid int64
	if err := d.QueryRow(`SELECT id FROM projects LIMIT 1`).Scan(&pid); err != nil {
		fmt.Println("pid:", err)
		os.Exit(1)
	}

	notes := []struct{ title, body string }{
		{"重试与退避实践", "重试要带指数退避和抖动，避免同步风暴。退避基数与最大次数需配置。"},
		{"幂等键设计", "幂等键保证同一次调用只产生一次副作用；重试与幂等是一对。"},
		{"schema 迁移流程", "迁移按版本顺序执行，写入 app_config 的 schema_version；大批量变更用 VACUUM INTO 做副本演练。"},
		{"证据预算规则", "问答取证受条数、字符、截止时间三重预算约束，超预算即截断。"},
		{"标签规范化规则", "标签统一小写去空格，逗号分隔；历史多仓库元数据已被清理。"},
		{"编译器与人在环", "编译器只写 pending 页，只有人批准后才进检索；lint 只产 todo。"},
		{"图感知取证机制", "一跳邻域遍历加个性化 PageRank 定序；矛盾关系不参与走查。"},
	}
	noteIDs := map[string]int64{}
	for _, n := range notes {
		nb, err := db.CreateNoteEx(d, pid, n.title, n.body, "", "knowledge", "manual")
		if err != nil {
			fmt.Println("note:", err)
			os.Exit(1)
		}
		noteIDs[n.title] = nb.ID
	}

	page := func(slug, title, body string) int64 {
		p, err := db.CreateWikiPage(d, db.WikiKindEntity, slug, title, pid, body)
		if err != nil {
			fmt.Println("page:", err)
			os.Exit(1)
		}
		return p.ID
	}
	p1 := page("alpha-hub", "Alpha Hub", "alpha 重试与路由的规则正文，重试策略在此：所有外部调用必须可重试、路由规则集中管理。")
	p2 := page("omega-dep", "Omega Dep", "omega 依赖说明，与问题词完全不同：该模块只描述内部依赖方向。")
	p3 := page("retry-detail", "Retry Detail", "重试策略细节：指数退避、最大重试次数、抖动范围。")
	p4 := page("backoff", "Backoff", "退避算法：基数翻倍直到上限，配随机抖动。")
	p5 := page("evidence-budget", "Evidence Budget", "证据预算：条数上限、字符上限、三秒截止时间。")
	p6 := page("migration-playbook", "Migration Playbook", "迁移手册：版本顺序、副本演练、回滚预案。")
	p8 := page("contradiction-only", "Contradiction Only", "重试矛盾的另一种主张：有观点认为不应默认重试，需业务确认。")
	p9 := page("idempotency", "Idempotency Keys", "幂等键保证同一次调用只产生一次副作用。")
	p10 := page("circuit-breaker", "Circuit Breaker", "熔断器在连续失败后断开调用，防止故障蔓延，是韧性设计的一环。")
	if _, err := db.CreateWikiPageAs(d, db.WikiKindEntity, "pending-draft", "Pending Draft", pid,
		"重试草稿：待批准的自动编译页。", db.WikiStatusPending, "wiki-compile"); err != nil {
		fmt.Println("pending page:", err)
		os.Exit(1)
	}

	link := func(from, to int64, rel string) {
		if err := db.LinkWikiPages(d, from, to, rel); err != nil {
			fmt.Println("link:", err)
			os.Exit(1)
		}
	}
	link(p1, p2, db.RelationRef)          // the blind-spot edge
	link(p3, p1, db.RelationPartOf)       // retry-detail belongs to the hub
	link(p4, p1, db.RelationPartOf)       // backoff belongs to the hub
	link(p1, p9, db.RelationRef)          // hub references idempotency
	link(p1, p10, db.RelationRef)         // hub references the breaker: vocabulary-disjoint answer
	link(p5, p3, db.RelationRef)          // budget references retry detail
	link(p6, p5, db.RelationDepends)      // playbook depends on budget
	link(p1, p8, db.RelationContradicts)  // never walked
	link(p3, p1, db.RelationRef)          // extra edge that the v21 merge keeps

	// ---- Downgrade page_links to its pre-v21 shape and re-spell every edge the
	// way a v20 database actually had it: free text, no CHECK. The duplicate
	// pair (hub->backoff as both cites and compiled-from) both fold onto ref,
	// so the migration has a real merge to prove rather than a no-op copy.
	if _, err := d.Exec(`DROP TABLE page_links`); err != nil {
		fmt.Println("drop:", err)
		os.Exit(1)
	}
	if _, err := d.Exec(`CREATE TABLE page_links (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_page_id INTEGER NOT NULL,
		to_page_id INTEGER NOT NULL,
		relation TEXT NOT NULL DEFAULT 'compiled-from',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(from_page_id, to_page_id, relation)
	)`); err != nil {
		fmt.Println("create:", err)
		os.Exit(1)
	}
	legacy := []struct {
		from, to int64
		rel      string
	}{
		{p1, p2, "compiled-from"},
		{p3, p1, "part_of"},
		{p4, p1, "references"},
		{p1, p9, "cites"},
		{p1, p10, "see-also"},
		{p5, p3, "related-to"},
		{p6, p5, "depends-on"},
		{p1, p8, "conflicts-with"},
		{p3, p1, "cites"},
		{p1, p4, "cites"},
		{p1, p4, "compiled-from"}, // same pair, two spellings, both -> ref
		{p3, p1, "explains"},      // model coinage: unknown -> ref (third spelling of that pair)
	}
	for _, e := range legacy {
		if _, err := d.Exec(`INSERT INTO page_links (from_page_id, to_page_id, relation) VALUES (?,?,?)`,
			e.from, e.to, e.rel); err != nil {
			fmt.Println("legacy edge:", err)
			os.Exit(1)
		}
	}
	if _, err := d.Exec(`UPDATE app_config SET value='20' WHERE key='schema_version'`); err != nil {
		fmt.Println("stamp:", err)
		os.Exit(1)
	}

	fmt.Println("seeded", path)
	fmt.Println("notes:", noteIDs)
	fmt.Println("pages:", map[string]int64{
		"alpha-hub": p1, "omega-dep": p2, "retry-detail": p3, "backoff": p4,
		"evidence-budget": p5, "migration-playbook": p6, "contradiction-only": p8, "idempotency": p9,
	})
}
