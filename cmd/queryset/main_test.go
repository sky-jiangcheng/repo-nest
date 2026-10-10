package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"repo-nest/internal/db"
)

// setupDB mirrors internal/db's own test helper: this is package main, so those
// helpers live in another package and cannot be reused.
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func makeProject(t *testing.T, database *sql.DB, name string) int64 {
	t.Helper()
	res, err := database.Exec(`INSERT INTO projects (name, root_path) VALUES (?, ?)`, name, "/tmp/"+name)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// captureStdout redirects os.Stdout for the duration of fn. dumpWorksheet writes
// to stdout by design (it is meant to be piped into an editor), so testing it
// means intercepting the real file handle rather than injecting a writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stdout = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// cmd/queryset exists because the labeled query set ADR-0014 决策 3 asks for
// cannot be written by a tool: relevance is a judgement about content. These
// tests cover the mechanical half — the numbering, the id resolution, and the
// refusal to emit a set that would make the gate lie.

func TestQuerysetSeedNumbersNotesStably(t *testing.T) {
	database := setupDB(t)
	pid := makeProject(t, database, "qs")
	for _, title := range []string{"重试策略", "幂等键", "超时约定"} {
		if _, err := db.CreateNoteEx(database, pid, title, "正文", "", "knowledge", "manual"); err != nil {
			t.Fatal(err)
		}
	}
	other := makeProject(t, database, "other")
	if _, err := db.CreateNoteEx(database, other, "别处的一条", "正文", "", "knowledge", "manual"); err != nil {
		t.Fatal(err)
	}

	ws := captureStdout(t, func() {
		if err := dumpWorksheet(database); err != nil {
			t.Fatalf("seed: %v", err)
		}
	})

	// Every note appears exactly once, numbered from 1 with no gaps — a gap or a
	// repeat would make the user's line numbers point at the wrong note.
	seen := map[int]string{}
	for _, line := range strings.Split(ws, "\n") {
		if !strings.HasPrefix(line, "# ") || !strings.Contains(line, "(id=") {
			continue
		}
		num, title := parseSeedLine(t, line)
		if prev, dup := seen[num]; dup {
			t.Errorf("note number %d used twice: %q and %q", num, prev, title)
		}
		seen[num] = title
	}
	if len(seen) != 4 {
		t.Fatalf("worksheet lists %d notes, want 4: %v", len(seen), seen)
	}
	for i := 1; i <= 4; i++ {
		if _, ok := seen[i]; !ok {
			t.Errorf("note number %d is missing from the worksheet", i)
		}
	}
	// The template must be present and must be a comment, or a user who runs
	// convert() without editing gets a confusing parse error instead of "no cases".
	if !strings.Contains(ws, `{"query": "", "relevant": []}`) {
		t.Error("worksheet has no template line to copy")
	}
}

// The number the user labels with must resolve to the note that number names.
// This is the whole value of the tool: the edit is a number, never a raw id.
func TestQuerysetEmitResolvesLineNumbersToIDs(t *testing.T) {
	database := setupDB(t)
	pid := makeProject(t, database, "qs")
	ids := map[string]int64{}
	for _, title := range []string{"重试策略", "幂等键"} {
		n, err := db.CreateNoteEx(database, pid, title, "正文", "", "knowledge", "manual")
		if err != nil {
			t.Fatal(err)
		}
		ids[title] = n.ID
	}

	ws := captureStdout(t, func() {
		if err := dumpWorksheet(database); err != nil {
			t.Fatal(err)
		}
	})
	edited := ws + "\n" + `{"query": "重试几次", "relevant": [1]}` + "\n"

	path := filepath.Join(t.TempDir(), "ws.tsv")
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "queries.jsonl")
	if err := convert(path, out); err != nil {
		t.Fatalf("emit: %v", err)
	}

	cases := readCases(t, out)
	if len(cases) != 1 {
		t.Fatalf("emitted %d cases, want 1", len(cases))
	}
	if cases[0]["query"] != "重试几次" {
		t.Errorf("query = %q", cases[0]["query"])
	}
	got := cases[0]["relevant"].([]any)
	if len(got) != 1 || int64(got[0].(float64)) != ids["重试策略"] {
		t.Errorf("relevant = %v, want [%d] (note number 1)", got, ids["重试策略"])
	}
}

// A case with no labels is skipped by the scorers, so accepting it would emit a
// set whose n= disagrees with the worksheet — which reads as a tool bug.
func TestQuerysetEmitRefusesUnlabeledOrBadInput(t *testing.T) {
	database := setupDB(t)
	pid := makeProject(t, database, "qs")
	if _, err := db.CreateNoteEx(database, pid, "唯一一条", "正文", "", "knowledge", "manual"); err != nil {
		t.Fatal(err)
	}
	ws := captureStdout(t, func() {
		if err := dumpWorksheet(database); err != nil {
			t.Fatal(err)
		}
	})

	cases := []struct {
		name string
		line string
		want string
	}{
		{"empty relevant", `{"query": "问题", "relevant": []}`, "no labels"},
		{"empty query", `{"query": "", "relevant": [1]}`, "empty query"},
		{"unknown note number", `{"query": "问题", "relevant": [99]}`, "not in this database"},
		{"unknown page number", `{"query": "问题", "relevant": [1], "relevant_pages": [99]}`, "page number"},
		{"not json", `这行不是 JSON`, "not valid JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ws.tsv")
			if err := os.WriteFile(path, []byte(ws+"\n"+tc.line+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := convert(path, filepath.Join(t.TempDir(), "out.jsonl"))
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// A database with no notes has nothing to label against; saying so beats
// emitting a worksheet with an empty list.
func TestQuerysetSeedRefusesAnEmptyDatabase(t *testing.T) {
	database := setupDB(t)
	if err := dumpWorksheet(database); err == nil {
		t.Error("expected a refusal for a database with no notes")
	} else if !strings.Contains(err.Error(), "no notes") {
		t.Errorf("error = %q, want it to say there are no notes", err)
	}
}

func parseSeedLine(t *testing.T, line string) (int, string) {
	t.Helper()
	rest := strings.TrimPrefix(line, "# ")
	parts := strings.Split(rest, "\t")
	if len(parts) < 2 {
		t.Fatalf("seed line %q has no tab-separated number", line)
	}
	num, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		t.Fatalf("seed line %q has a non-numeric number: %v", line, err)
	}
	// parts[1:] is "<project>\t<title>"; rejoin in case a title contains a tab.
	return num, strings.Join(parts[1:], "\t")
}

// The emitted file has to be readable by the tool that consumes it. cmd/abeval
// parses {"query","relevant"}; abeval.Case has no json tags, so encoding it
// directly would produce {"Query","Relevant"} and score zero cases against a
// perfectly good gate. This asserts the wire shape by decoding with the same
// struct abeval uses.
func TestQuerysetEmitMatchesTheAbevalWireFormat(t *testing.T) {
	database := setupDB(t)
	pid := makeProject(t, database, "qs")
	note, err := db.CreateNoteEx(database, pid, "重试策略", "正文", "", "knowledge", "manual")
	if err != nil {
		t.Fatal(err)
	}
	want := note.ID
	ws := captureStdout(t, func() {
		if err := dumpWorksheet(database); err != nil {
			t.Fatal(err)
		}
	})
	path := filepath.Join(t.TempDir(), "ws.tsv")
	if err := os.WriteFile(path, []byte(ws+"\n"+`{"query": "重试几次", "relevant": [1]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "queries.jsonl")
	if err := convert(path, out); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// Exactly the shape cmd/abeval's labeledCase declares, lower-cased.
	var decoded struct {
		Query    string  `json:"query"`
		Relevant []int64 `json:"relevant"`
	}
	line := strings.TrimSpace(string(raw))
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("abeval cannot decode the emitted line %q: %v", line, err)
	}
	if decoded.Query != "重试几次" {
		t.Errorf("query = %q", decoded.Query)
	}
	if len(decoded.Relevant) != 1 || decoded.Relevant[0] != want {
		t.Errorf("relevant = %v, want [%d]", decoded.Relevant, want)
	}
}

func readCases(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("emitted line is not JSON: %q", line)
		}
		out = append(out, m)
	}
	return out
}

// The page arm (abeval -pages) labels wiki_page ids under "relevant_pages" on
// the same worksheet. Seed must list the approved pages as numbered candidates
// — approved only, because a pending page can never be retrieved as evidence
// and labeling one would be wasted judgment — and the template must carry the
// second key.
func TestQuerysetSeedListsApprovedPagesForThePageArm(t *testing.T) {
	database := setupDB(t)
	pid := makeProject(t, database, "qs")
	if _, err := db.CreateNoteEx(database, pid, "重试策略", "正文", "", "knowledge", "manual"); err != nil {
		t.Fatal(err)
	}
	approved, err := db.CreateWikiPage(database, db.WikiKindEntity, "alpha-hub", "Alpha Hub", pid, "正文")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := db.CreateWikiPageAs(database, db.WikiKindEntity, "omega-draft", "Omega Draft", pid,
		"草稿", db.WikiStatusPending, "wiki-compile")
	if err != nil {
		t.Fatal(err)
	}

	ws := captureStdout(t, func() {
		if err := dumpWorksheet(database); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(ws, "页面清单") {
		t.Fatalf("worksheet has no page list:\n%s", ws)
	}
	if !strings.Contains(ws, `"relevant_pages"`) {
		t.Errorf("template line does not mention relevant_pages")
	}
	if !strings.Contains(ws, fmt.Sprintf("(id=%d)", approved.ID)) {
		t.Errorf("approved page id=%d not listed as a candidate", approved.ID)
	}
	if strings.Contains(ws, fmt.Sprintf("(id=%d)", pending.ID)) {
		t.Error("a pending page was listed; evidence can never retrieve it")
	}
}

// Page numbers resolve through the PAGE list, not the note list — both lists
// start at 1, so this is exactly the collision the section split exists for.
// A page-only line (no relevant) is legal: the note arm skips unlabeled cases
// and the page arm needs this to measure without note labels.
func TestQuerysetEmitResolvesPageNumbersToRelevantPages(t *testing.T) {
	database := setupDB(t)
	pid := makeProject(t, database, "qs")
	note, err := db.CreateNoteEx(database, pid, "重试策略", "正文", "", "knowledge", "manual")
	if err != nil {
		t.Fatal(err)
	}
	page, err := db.CreateWikiPage(database, db.WikiKindEntity, "alpha-hub", "Alpha Hub", pid, "正文")
	if err != nil {
		t.Fatal(err)
	}

	ws := captureStdout(t, func() {
		if err := dumpWorksheet(database); err != nil {
			t.Fatal(err)
		}
	})
	edited := ws + "\n" +
		`{"query": "两臂都标", "relevant": [1], "relevant_pages": [1]}` + "\n" +
		`{"query": "只标页", "relevant_pages": [1]}` + "\n"
	path := filepath.Join(t.TempDir(), "ws.tsv")
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "queries.jsonl")
	if err := convert(path, out); err != nil {
		t.Fatalf("emit: %v", err)
	}

	cases := readCases(t, out)
	if len(cases) != 2 {
		t.Fatalf("emitted %d cases, want 2", len(cases))
	}
	rp := cases[0]["relevant_pages"].([]any)
	if len(rp) != 1 || int64(rp[0].(float64)) != page.ID {
		t.Errorf("relevant_pages = %v, want [%d] (page number 1, not the note)", rp, page.ID)
	}
	r := cases[0]["relevant"].([]any)
	if len(r) != 1 || int64(r[0].(float64)) != note.ID {
		t.Errorf("relevant = %v, want [%d]", r, note.ID)
	}
	rp1 := cases[1]["relevant_pages"].([]any)
	if len(rp1) != 1 || int64(rp1[0].(float64)) != page.ID {
		t.Errorf("page-only case relevant_pages = %v, want [%d]", rp1, page.ID)
	}
	if v, ok := cases[1]["relevant"]; ok {
		if arr, isArr := v.([]any); !isArr || len(arr) != 0 {
			t.Errorf("page-only case emitted relevant = %v, want the key absent or empty", v)
		}
	}
}

// A note-only database keeps the old worksheet shape: no page list, no
// relevant_pages in the template — emit stays byte-identical to before.
func TestQuerysetSeedWithoutPagesKeepsTheOldShape(t *testing.T) {
	database := setupDB(t)
	pid := makeProject(t, database, "qs")
	if _, err := db.CreateNoteEx(database, pid, "重试策略", "正文", "", "knowledge", "manual"); err != nil {
		t.Fatal(err)
	}
	ws := captureStdout(t, func() {
		if err := dumpWorksheet(database); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(ws, "页面清单") || strings.Contains(ws, "relevant_pages") {
		t.Errorf("page machinery leaked into a page-less worksheet:\n%s", ws)
	}
}
