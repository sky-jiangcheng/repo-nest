package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"repo-nest/internal/db"
)

// ADR-0016 待决 ① resolved as "yes": the lint model pass joins the compile job
// table instead of getting a second state machine. These tests pin the parts
// that decision actually rests on — one queue, one polling surface, one crash
// recovery path, and no second implementation of either kernel.

// lintJobProject seeds the fixture and returns a project id that owns pages, since
// a queued lint is per-project (db.CreateLintJob).
func lintJobProject(t *testing.T, svc *Service) (int64, map[string]int64) {
	t.Helper()
	ids := lintFixture(t, svc)
	return mustProjectID(t, svc, "lint"), ids
}

func configureLintJob(t *testing.T, svc *Service, url string) {
	t.Helper()
	for k, v := range map[string]string{
		"ai_chat_base_url": url + "/v1",
		"ai_chat_model":    "stub",
		wikiLintLLMKey:     "1",
	} {
		if err := svc.UpdateConfig(k, v); err != nil {
			t.Fatalf("config %s: %v", k, err)
		}
	}
}

// Submission must return without touching the model, for the same reason compile
// jobs do: the pass runs under a 10-minute ceiling.
func TestStartLintJob_ReturnsImmediately(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := lintJobProject(t, svc)
	srv, calls := slowChatStub(t, 400*time.Millisecond, "[]")
	configureLintJob(t, svc, srv.URL)

	start := time.Now()
	id, err := svc.StartLintJob(pid)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("submit blocked for %v; it must return before the model is called", elapsed)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("model was called %d time(s) during submission, want 0", n)
	}
	j, err := db.GetCompileJob(svc.db, id)
	if err != nil {
		t.Fatal(err)
	}
	if j.Kind != db.JobKindLint {
		t.Errorf("Kind = %q, want %q", j.Kind, db.JobKindLint)
	}
	if j.Status != db.CompileJobQueued {
		t.Errorf("Status = %q, want queued", j.Status)
	}
	if j.Requested <= 0 {
		t.Errorf("Requested = %d, want the page count", j.Requested)
	}
}

// The gate is the point of the feature: the pass ships page content to a model,
// so a queued job that would silently do nothing is worse than a refusal.
func TestStartLintJob_RefusesWithoutTheOptIn(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := lintJobProject(t, svc)
	srv, calls := slowChatStub(t, 0, "[]")
	configureLintJob(t, svc, srv.URL)
	if err := svc.UpdateConfig(wikiLintLLMKey, "0"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartLintJob(pid); err == nil {
		t.Error("expected a refusal while the model pass is switched off")
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("model was called %d time(s) on a refused submit, want 0", n)
	}
	jobs, err := svc.ListCompileJobs(0, db.JobKindLint, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Errorf("a refused submit enqueued %d job(s), want 0", len(jobs))
	}
}

func TestStartLintJob_RefusesWithoutPagesOrEndpoint(t *testing.T) {
	svc, _ := setupService(t)
	srv, _ := slowChatStub(t, 0, "[]")
	configureLintJob(t, svc, srv.URL)

	// No project: a job row is always scoped to one.
	if _, err := svc.StartLintJob(0); err == nil {
		t.Error("expected a refusal without a project id")
	}

	pid, _ := lintJobProject(t, svc)
	// Pages exist, so the refusal has to come from somewhere else: clear them
	// and an empty pass must not get a job row.
	if _, err := svc.db.Exec("DELETE FROM wiki_pages"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartLintJob(pid); err == nil {
		t.Error("expected a refusal when there are no pages to lint")
	}

	lintJobProject(t, svc)
	if err := svc.UpdateConfig("ai_chat_model", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartLintJob(pid); err == nil {
		t.Error("expected a refusal when no model is configured")
	}
}

// The worker must dispatch on the row's kind, and the lint kernel must be the
// existing synchronous pass rather than a second implementation.
func TestRunNextJob_DrainsALintJob(t *testing.T) {
	svc, _ := setupService(t)
	pid, ids := lintJobProject(t, svc)
	srv, calls := slowChatStub(t, 0, fmt.Sprintf(
		`[{"type":"contradiction","page_id":%d,"other_id":%d,"detail":"两页对超时时间陈述冲突"}]`,
		ids["auth-module"], ids["payment-gateway-retry"]))
	configureLintJob(t, svc, srv.URL)

	jobID, err := svc.StartLintJob(pid)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.runNextCompileJob(context.Background()) {
		t.Fatal("worker claimed nothing")
	}
	if calls.Load() == 0 {
		t.Error("the lint job never reached the model")
	}
	j, err := db.GetCompileJob(svc.db, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != db.CompileJobDone {
		t.Fatalf("status = %q (error=%q), want succeeded", j.Status, j.Error)
	}
	if j.Findings == 0 {
		t.Error("Findings = 0, want the model's finding counted")
	}
	if j.NotesTotal == 0 {
		t.Error("NotesTotal = 0, want the page count for the progress display")
	}
	// Structural checks still run in the same pass, so the row is not only the
	// model half — the counter has to reflect both.
	if j.NotesDone == 0 {
		t.Error("NotesDone = 0, want the pages actually checked")
	}
}

// A compile job and a lint job share the table; they must stay distinguishable
// through every read path, or the panel renders one job's counters under the
// other's labels.
func TestListCompileJobs_FiltersByKind(t *testing.T) {
	svc, _ := setupService(t)
	pid := jobCorpus(t, svc)
	lintFixture(t, svc)
	srv, _ := slowChatStub(t, 0, "[]")
	configureCompile(t, svc, srv.URL)
	configureLintJob(t, svc, srv.URL)

	compileID, err := db.CreateCompileJob(svc.db, pid, 2)
	if err != nil {
		t.Fatal(err)
	}
	lintPID := mustProjectID(t, svc, "lint")
	lintID, err := db.CreateLintJob(svc.db, lintPID, 3)
	if err != nil {
		t.Fatal(err)
	}

	all, err := svc.ListCompileJobs(0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("unfiltered list = %d jobs, want 2", len(all))
	}

	onlyLint, err := svc.ListCompileJobs(0, db.JobKindLint, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyLint) != 1 || onlyLint[0].ID != lintID {
		t.Errorf("lint list = %+v, want just job %d", onlyLint, lintID)
	}
	onlyCompile, err := svc.ListCompileJobs(0, db.JobKindCompile, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyCompile) != 1 || onlyCompile[0].ID != compileID {
		t.Errorf("compile list = %+v, want just job %d", onlyCompile, compileID)
	}
	// An unknown kind is a caller bug, not an empty result.
	if _, err := svc.ListCompileJobs(0, "nonsense", 10); err == nil {
		t.Error("expected an error for an unknown job kind")
	}
}

// A job row is always scoped to one project: project_id is NOT NULL under a
// foreign key, so "every project" has no representation and must not be faked
// with a sentinel project.
func TestCreateLintJob_RequiresAProject(t *testing.T) {
	svc, _ := setupService(t)
	seedProject(t, svc.db, "p", "/tmp/p")
	if _, err := db.CreateLintJob(svc.db, 0, 5); err == nil {
		t.Error("a lint job with no project must be refused")
	}
	if _, err := db.CreateLintJob(svc.db, -1, 5); err == nil {
		t.Error("a lint job with a negative project id must be refused")
	}
	pid := mustProjectID(t, svc, "p")
	if _, err := db.CreateLintJob(svc.db, pid, 5); err != nil {
		t.Errorf("a lint job for a real project must be allowed: %v", err)
	}
	if _, err := db.CreateCompileJob(svc.db, 0, 5); err == nil {
		t.Error("a compile job with no project must be refused")
	}
}

// One recovery path for both kinds: work left running by a dead process is
// marked failed, never resumed, because the stopping point is not recorded.
func TestRecoverInterruptedJobs_CoversBothKinds(t *testing.T) {
	svc, _ := setupService(t)
	pid := jobCorpus(t, svc)
	compileID, err := db.CreateCompileJob(svc.db, pid, 1)
	if err != nil {
		t.Fatal(err)
	}
	lintID, err := db.CreateLintJob(svc.db, pid, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{compileID, lintID} {
		if _, err := svc.db.Exec(
			`UPDATE compile_jobs SET status='running', started_at=CURRENT_TIMESTAMP WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}
	svc.recoverInterruptedCompileJobs()
	for _, id := range []int64{compileID, lintID} {
		j, err := db.GetCompileJob(svc.db, id)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status != db.CompileJobFailed {
			t.Errorf("job %d status = %q, want failed", id, j.Status)
		}
		if j.Error == "" {
			t.Errorf("job %d has no reason recorded", id)
		}
	}
}

func TestCancelCompileJob_StopsALintJobBeforeItRuns(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := lintJobProject(t, svc)
	srv, calls := slowChatStub(t, 0, "[]")
	configureLintJob(t, svc, srv.URL)

	jobID, err := svc.StartLintJob(pid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelCompileJob(jobID); err != nil {
		t.Fatal(err)
	}
	svc.runNextCompileJob(context.Background())
	j, err := db.GetCompileJob(svc.db, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != db.CompileJobCanceled {
		t.Errorf("status = %q, want canceled", j.Status)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("a canceled lint job still called the model %d time(s)", n)
	}
}

// A worker shutting down must not leave a lint job stuck in 'running'.
func TestRunNextJob_LintJobCanceledOnShutdown(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := lintJobProject(t, svc)
	srv, _ := slowChatStub(t, 0, "[]")
	configureLintJob(t, svc, srv.URL)

	jobID, err := svc.StartLintJob(pid)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already shut down
	svc.runNextCompileJob(ctx)

	j, err := db.GetCompileJob(svc.db, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != db.CompileJobCanceled {
		t.Errorf("status = %q, want canceled", j.Status)
	}
}

// A project filter must not leak another project's jobs into the panel.
func TestListCompileJobs_ProjectFilterScopesLintJobs(t *testing.T) {
	svc, _ := setupService(t)
	scoped := seedProject(t, svc.db, "scoped", "/tmp/scoped")
	other := seedProject(t, svc.db, "other", "/tmp/other")
	if _, err := db.CreateLintJob(svc.db, scoped, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateLintJob(svc.db, other, 2); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListCompileJobs(scoped, db.JobKindLint, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ProjectID != scoped {
		t.Errorf("project-scoped list = %+v, want only the job for %d", got, scoped)
	}
	all, err := svc.ListCompileJobs(0, db.JobKindLint, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("unfiltered lint list = %d jobs, want 2", len(all))
	}
}
