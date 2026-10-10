// Command queryset helps build the labeled query set ADR-0014 决策 3 requires
// before W2's evidence retrieval can be measured on a real corpus.
//
// The measurement half already exists: internal/search/abeval scores Recall@k /
// NDCG@k over labeled cases, and cmd/abeval runs it against the live database.
// What does not exist is the labeled set itself, and the reason it has not been
// written by hand is not laziness — it is that producing it requires reading
// notes and judging which ones answer a question, and no tool can do that part.
// Writing ids blind (query the DB, guess which rows matter) produces a set that
// measures the guesser.
//
// So this command does the mechanical half only:
//
//	-seed        dump every note as a numbered candidate, so labeling is a
//	             decision about content rather than a hunt for ids
//	-emit        turn a partially filled answer file into queries.jsonl, with
//	             the id lookup done for the user
//
// The user still decides relevance. That is the point: a fabricated label set
// would make the gate pass or fail for reasons that have nothing to do with
// whether retrieval is good.
//
// Usage:
//
//	reponest-queryset -db path/to/dashboard.db -seed > notes.tsv
//	# edit notes.tsv: for each query line, list the note numbers that answer it
//	#   (and, when the database has approved wiki pages, the page numbers under
//	#   relevant_pages — the same numbers the 页面清单 section printed)
//	reponest-queryset -db path/to/dashboard.db -emit notes.tsv -out queries.jsonl
//	reponest-abeval -cases queries.jsonl -k 10 -min-recall 0.05
//	reponest-abeval -pages -cases queries.jsonl -k 10 -min-recall 0.05
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"repo-nest/internal/db"
	"repo-nest/internal/platform"
)

func main() {
	platform.SetPrivateUmask()

	dbPath := flag.String("db", "", "database path (default: the app's DB)")
	seed := flag.Bool("seed", false, "dump notes as a labeling worksheet")
	emit := flag.String("emit", "", "convert a filled worksheet into queries.jsonl")
	out := flag.String("out", "", "output path for -emit (default: stdout)")
	flag.Parse()

	if !*seed && *emit == "" {
		log.Fatal("queryset: need -seed or -emit")
	}

	path := *dbPath
	if path == "" {
		path = platform.GetDbPath()
	}
	database, err := db.InitDB(path)
	if err != nil {
		log.Fatalf("queryset: open db: %v", err)
	}
	defer database.Close()

	if *seed {
		if err := dumpWorksheet(database); err != nil {
			log.Fatalf("queryset: seed: %v", err)
		}
		return
	}
	if err := convert(*emit, *out); err != nil {
		log.Fatalf("queryset: emit: %v", err)
	}
}

type noteRow struct {
	num     int
	id      int64
	project string
	title   string
}

// dumpWorksheet prints notes with stable line numbers. The numbers are the unit
// the user labels with, because an id is unforgiving to hand-edit and a title
// is not unique.
func dumpWorksheet(database *sql.DB) error {
	projects, err := db.GetAllProjects(database)
	if err != nil {
		return err
	}
	names := make(map[int64]string, len(projects))
	for _, p := range projects {
		names[p.ID] = p.Name
	}

	// Approved pages only: evidence can never retrieve a pending page, so
	// listing one would invite the user to spend judgment on a label the page
	// arm can never use.
	pages, err := db.ListWikiPagesByStatus(database, db.WikiStatusApproved, 0)
	if err != nil {
		return fmt.Errorf("list pages: %w", err)
	}
	hasPages := len(pages) > 0

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	fmt.Fprintln(w, "# RepoNest 检索标注工作表")
	fmt.Fprintln(w, "#")
	fmt.Fprintln(w, "# 步骤 1：给每条笔记起一个真实的问题——你确实会问的问题。")
	fmt.Fprintln(w, "#         已有的问题比编造的更真实；只写你真会问的。")
	fmt.Fprintln(w, "# 步骤 2：对每个问题，在 relevant= 里填能真正回答它的笔记行号。")
	fmt.Fprintln(w, "#         填 1-3 条即可。凑数会把门禁的结论变成噪音。")
	if hasPages {
		fmt.Fprintln(w, "#         若下面的页面清单里有能回答它的 wiki 页，把页行号填进 relevant_pages=。")
	}
	fmt.Fprintln(w, "# 步骤 3：保存后运行 reponest-queryset -emit <本文件> -out queries.jsonl")
	fmt.Fprintln(w, "#")
	fmt.Fprintln(w, "# ---- 笔记清单 ----")

	var rows []noteRow
	for pid := range names {
		notes, err := db.ListNotes(database, pid)
		if err != nil {
			return fmt.Errorf("list notes for project %d: %w", pid, err)
		}
		for _, n := range notes {
			rows = append(rows, noteRow{num: len(rows) + 1, id: n.ID, project: names[pid], title: n.Title})
		}
	}
	if len(rows) == 0 {
		return fmt.Errorf("no notes in this database — nothing to label against")
	}
	for _, r := range rows {
		fmt.Fprintf(w, "# %d\t%s\t%s\t(id=%d)\n", r.num, r.project, r.title, r.id)
	}

	if hasPages {
		fmt.Fprintln(w, "#")
		fmt.Fprintln(w, "# ---- 页面清单（abeval -pages 用；只列已批准页，pending 页不能作证据）----")
		for i, p := range pages {
			proj := "全局"
			if p.ProjectID != 0 {
				proj = names[p.ProjectID]
				if proj == "" {
					proj = fmt.Sprintf("项目%d", p.ProjectID)
				}
			}
			fmt.Fprintf(w, "# %d\t%s\t%s\t%s\t(id=%d)\n", i+1, proj, p.Slug, p.Title, p.ID)
		}
	}

	fmt.Fprintln(w, "#")
	fmt.Fprintln(w, "# ---- 待标注 ----")
	if hasPages {
		fmt.Fprintln(w, `# {"query": "", "relevant": [], "relevant_pages": []}`)
		fmt.Fprintln(w, "# 逐行复制上面那条模板并填写；query 不能为空，relevant 与 relevant_pages 至少填一个")
		fmt.Fprintln(w, "# （relevant= 笔记清单行号，relevant_pages= 页面清单行号——两个清单都从 1 开始）。")
	} else {
		fmt.Fprintln(w, `# {"query": "", "relevant": []}`)
		fmt.Fprintln(w, "# 逐行复制上面那条模板并填写；query 不能为空，relevant 至少一个行号。")
	}
	return nil
}

// convert reads the worksheet back and writes queries.jsonl. Note and page
// numbers are resolved to real ids here, so the user's edit is a number and
// never a raw id they could mistype. Each case carries whichever of
// relevant (notes) / relevant_pages (wiki pages) was labeled; a line with
// neither is refused.
func convert(in, out string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()

	noteIDs, pageIDs, err := worksheetLists(in)
	if err != nil {
		return err
	}

	type emitted struct {
		query string
		notes []int64
		pages []int64
	}
	var cases []emitted
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var c struct {
			Query         string `json:"query"`
			Relevant      []int  `json:"relevant"`
			RelevantPages []int  `json:"relevant_pages"`
		}
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return fmt.Errorf("line %d is not valid JSON: %w", lineNo, err)
		}
		if strings.TrimSpace(c.Query) == "" {
			return fmt.Errorf("line %d has an empty query", lineNo)
		}
		if len(c.Relevant) == 0 && len(c.RelevantPages) == 0 {
			// A case with no labels is skipped by the scorers, so accepting it
			// silently would produce a report whose n= does not match the
			// worksheet — which reads as a bug in the tool.
			return fmt.Errorf("line %d (%q) has no labels (relevant / relevant_pages); drop the line or label it", lineNo, c.Query)
		}
		e := emitted{query: strings.TrimSpace(c.Query)}
		for _, num := range c.Relevant {
			id, ok := noteIDs[num]
			if !ok {
				return fmt.Errorf("line %d references note number %d, which is not in this database's list", lineNo, num)
			}
			e.notes = append(e.notes, id)
		}
		for _, num := range c.RelevantPages {
			id, ok := pageIDs[num]
			if !ok {
				return fmt.Errorf("line %d references page number %d, which is not in this database's page list", lineNo, num)
			}
			e.pages = append(e.pages, id)
		}
		cases = append(cases, e)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(cases) == 0 {
		return fmt.Errorf("no labeled cases found — nothing to convert")
	}

	// How many the set can actually support: below this, a single query can move
	// recall by more than the gate's margin, so the verdict is noise.
	const minUseful = 10
	w := os.Stdout
	if out != "" {
		fh, err := os.Create(out)
		if err != nil {
			return err
		}
		defer fh.Close()
		w = fh
	}
	// abeval.Case has no json tags, so encoding it directly would emit
	// {"Query":...,"Relevant":...} — which cmd/abeval's labeledCase cannot read
	// back (it wants "query"/"relevant"), so the file would silently score zero
	// cases. Encode the wire shape explicitly; omitempty keeps a note-only set
	// byte-identical to the pre-page format.
	enc := json.NewEncoder(w)
	for _, c := range cases {
		if err := enc.Encode(struct {
			Query         string  `json:"query"`
			Relevant      []int64 `json:"relevant,omitempty"`
			RelevantPages []int64 `json:"relevant_pages,omitempty"`
		}{Query: c.query, Relevant: c.notes, RelevantPages: c.pages}); err != nil {
			return err
		}
	}
	verb := "written to stdout"
	if out != "" {
		verb = "written to " + out
	}
	fmt.Fprintf(os.Stderr, "queryset: %d case(s) %s\n", len(cases), verb)
	if len(cases) < minUseful {
		fmt.Fprintf(os.Stderr,
			"queryset: note — %d case(s) is a starting point, not evidence. At this size one\n"+
				"  query can move recall@10 by more than the gate margin, so run it for direction\n"+
				"  and keep labeling before treating GATE PASS as a result.\n", len(cases))
	}
	return nil
}

// worksheetLists re-reads the note list AND the page list from the worksheet
// itself. Taking the numbers from the same file the user edited is what makes
// them line up; looking them up again from the database would silently
// re-index if the db changed between the two steps. The two lists both start
// at 1, so which one a number belongs to is decided by the section header the
// line sits under — a worksheet without a 页面清单 section simply yields an
// empty page map (and page labels would then be refused as unknown numbers,
// not silently resolved against the note list).
func worksheetLists(path string) (noteIDs, pageIDs map[int]int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	noteIDs = map[int]int64{}
	pageIDs = map[int]int64{}
	section := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.Contains(line, "---- 笔记清单"):
			section = "notes"
			continue
		case strings.Contains(line, "---- 页面清单"):
			section = "pages"
			continue
		}
		if section == "" || !strings.HasPrefix(line, "# ") {
			continue
		}
		rest := strings.TrimPrefix(line, "# ")
		tab := strings.Index(rest, "\t")
		if tab < 0 {
			continue
		}
		num, err := strconv.Atoi(strings.TrimSpace(rest[:tab]))
		if err != nil {
			continue
		}
		open := strings.LastIndex(rest, "(id=")
		if open < 0 || !strings.HasSuffix(rest, ")") {
			continue
		}
		id, err := strconv.ParseInt(rest[open+len("(id="):len(rest)-1], 10, 64)
		if err != nil {
			continue
		}
		if section == "pages" {
			pageIDs[num] = id
		} else {
			noteIDs[num] = id
		}
	}
	return noteIDs, pageIDs, sc.Err()
}
