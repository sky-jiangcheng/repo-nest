package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"repo-nest/internal/db"
)

// serveMergeReply stands up a stub OpenAI-compatible endpoint returning body for
// every chat completion, and points the ai_chat_* keys at it.
func serveMergeReply(t *testing.T, svc *Service, body string) *atomic.Int64 {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"message": map[string]any{"content": body},
		}}})
		_, _ = io.WriteString(w, string(encoded))
	}))
	t.Cleanup(srv.Close)
	for k, v := range map[string]string{
		"ai_chat_base_url": srv.URL + "/v1",
		"ai_chat_model":    "stub",
		"ai_chat_api_key":  "k",
	} {
		if err := svc.UpdateConfig(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return &hits
}

func mergeSuggestPages(t *testing.T, svc *Service) []db.WikiPage {
	t.Helper()
	pages, err := db.ListWikiPages(svc.db, "", 0)
	if err != nil {
		t.Fatalf("ListWikiPages: %v", err)
	}
	return pages
}

// ADR-0019 晋升条件 4, first half: with wiki_merge_suggest off, no request may
// leave the app — the gate is checked before the endpoint is dialed.
func TestWikiMergeSuggest_OffByDefault(t *testing.T) {
	svc, _ := setupService(t)
	mergeFixture(t, svc)
	hits := serveMergeReply(t, svc, `[]`)

	findings, note := svc.runWikiMergeSuggest(0, mergeSuggestPages(t, svc))
	if len(findings) != 0 {
		t.Fatalf("findings with the gate off: %+v", findings)
	}
	if !strings.Contains(note, "wiki_merge_suggest") {
		t.Errorf("note = %q, want it to name the switch that is off", note)
	}
	if hits.Load() != 0 {
		t.Errorf("the endpoint was dialed with the gate off (%d hits)", hits.Load())
	}
}

// A valid same=true judgment becomes a `[lint] merge` finding naming BOTH
// directions, lands on the loser's project, and dedupes on the next run.
func TestWikiMergeSuggest_FilesMergeTodoAndDedupes(t *testing.T) {
	svc, _ := setupService(t)
	winner, loser, _, _ := mergeFixture(t, svc)
	hits := serveMergeReply(t, svc, fmt.Sprintf(
		`[{"a":"payment-gateway","b":"gateway-payment","same":true,"winner":"payment-gateway","reason":"两页是同一概念，来源更完整"},`+
			`{"a":"payment-gateway","b":"gateway-payment","same":false,"winner":"","reason":""}]`))
	if err := svc.UpdateConfig(wikiMergeSuggestKey, "1"); err != nil {
		t.Fatal(err)
	}

	// Through RunWikiLint so the findings-to-todos path is exercised too — with
	// wiki_lint_llm still off, the stub is reached only by the merge pass.
	rep, err := svc.RunWikiLint(0, true)
	if err != nil {
		t.Fatalf("RunWikiLint: %v", err)
	}
	if rep.MergeSuggestions != 1 {
		t.Fatalf("MergeSuggestions = %d, want 1 (note: %q)", rep.MergeSuggestions, rep.MergeSuggestNote)
	}
	if rep.MergeSuggestNote != "" {
		t.Errorf("MergeSuggestNote = %q, want empty on a clean run", rep.MergeSuggestNote)
	}
	if hits.Load() == 0 {
		t.Fatal("the stub endpoint was never reached; the pass silently no-op'd")
	}
	var found bool
	for _, f := range rep.Findings {
		if f.Check != "merge" {
			continue
		}
		found = true
		if f.PageID != loser {
			t.Errorf("finding filed on page %d, want the loser %d", f.PageID, loser)
		}
		if !strings.Contains(f.Detail, "[[payment-gateway]]") || !strings.Contains(f.Detail, "→") {
			t.Errorf("detail must name both directions for the human to flip: %q", f.Detail)
		}
	}
	if !found {
		t.Fatal("no merge finding in the report")
	}

	// The todo itself, then the dedupe: a stable title must not re-file.
	pid := mustProjectID(t, svc, "merge")
	var mergeTodos int
	for _, td := range svc.ListTodos(pid) {
		if strings.Contains(td.Title, "[lint] merge") {
			mergeTodos++
		}
	}
	if mergeTodos != 1 {
		t.Fatalf("merge todos = %d, want exactly 1", mergeTodos)
	}
	again, err := svc.RunWikiLint(0, true)
	if err != nil {
		t.Fatalf("second RunWikiLint: %v", err)
	}
	if again.TodosCreated != 0 {
		t.Errorf("second run created %d todos; the stable title must dedupe", again.TodosCreated)
	}
	_ = winner
}

// 晋升条件 4, second half: a fabricated slug voids the WHOLE reply, not just
// the item that fabricated it.
func TestWikiMergeSuggest_FabricatedSlugVoidsWholeReply(t *testing.T) {
	svc, _ := setupService(t)
	mergeFixture(t, svc)
	serveMergeReply(t, svc, `[{"a":"payment-gateway","b":"gateway-payment","same":true,"winner":"payment-gateway","reason":"真的一对"},`+
		`{"a":"payment-gateway","b":"made-up-slug","same":true,"winner":"made-up-slug","reason":"编造的一对"}]`)
	if err := svc.UpdateConfig(wikiMergeSuggestKey, "1"); err != nil {
		t.Fatal(err)
	}

	findings, note := svc.runWikiMergeSuggest(0, mergeSuggestPages(t, svc))
	if len(findings) != 0 {
		t.Fatalf("a fabricated slug must void the whole reply, got %+v", findings)
	}
	if !strings.Contains(note, "不采信") {
		t.Errorf("note = %q, want it to say the reply was not adopted", note)
	}
}

// A judgment about a pair that was never submitted is as untrusted as an
// invented slug: the winner check alone would let the model merge anything it
// can name.
func TestWikiMergeSuggest_WinnerOutsidePairVoidsReply(t *testing.T) {
	svc, _ := setupService(t)
	mergeFixture(t, svc)
	// retry-policy exists but was never paired with payment-gateway.
	serveMergeReply(t, svc, `[{"a":"payment-gateway","b":"retry-policy","same":true,"winner":"retry-policy","reason":"越界判定"}]`)
	if err := svc.UpdateConfig(wikiMergeSuggestKey, "1"); err != nil {
		t.Fatal(err)
	}

	findings, note := svc.runWikiMergeSuggest(0, mergeSuggestPages(t, svc))
	if len(findings) != 0 {
		t.Fatalf("an unsubmitted pair must void the reply, got %+v", findings)
	}
	if !strings.Contains(note, "未提交") {
		t.Errorf("note = %q, want it to say the pair was never submitted", note)
	}
}

// Non-JSON is the same refusal, with the same note discipline as the lint pass.
func TestWikiMergeSuggest_UnparseableReplyIsNotAdopted(t *testing.T) {
	svc, _ := setupService(t)
	mergeFixture(t, svc)
	serveMergeReply(t, svc, "这两页看着像同一个概念，建议合并")
	if err := svc.UpdateConfig(wikiMergeSuggestKey, "1"); err != nil {
		t.Fatal(err)
	}

	findings, note := svc.runWikiMergeSuggest(0, mergeSuggestPages(t, svc))
	if len(findings) != 0 {
		t.Fatalf("an unparseable reply must not produce findings, got %+v", findings)
	}
	if !strings.Contains(note, "无法解析") {
		t.Errorf("note = %q, want it to say the reply was unparseable", note)
	}
}

// The key is independent: authorizing merge suggestions must not silently
// authorize the page-reading contradiction pass, and vice versa.
func TestWikiMergeSuggest_KeyIsIndependentOfLintLLM(t *testing.T) {
	svc, _ := setupService(t)
	mergeFixture(t, svc)
	serveMergeReply(t, svc, `[]`)
	if err := svc.UpdateConfig(wikiMergeSuggestKey, "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartLintJob(mustProjectID(t, svc, "merge")); err != nil {
		t.Fatalf("merge-suggest alone must make the lint job worth queueing: %v", err)
	}
	// And the contradiction pass stays off with its own key unset.
	rep, err := svc.RunWikiLint(0, true)
	if err != nil {
		t.Fatalf("RunWikiLint: %v", err)
	}
	if rep.LLMRan {
		t.Error("wiki_lint_llm must stay off when only wiki_merge_suggest is set")
	}
	if !strings.Contains(rep.LLMNote, "wiki_lint_llm") {
		t.Errorf("LLMNote = %q, want it to name wiki_lint_llm", rep.LLMNote)
	}
}
