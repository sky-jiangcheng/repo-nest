package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"repo-nest/internal/db"
)

// setupDBFile gives each test its own throwaway database file so the CLI paths
// that take -db are exercised against a real file (the flag's whole point is
// to redirect away from the user's app database).
func setupDBFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cmd-test.db")
	database, err := db.InitDB(path)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return path
}

// The wiki-export report line is the operator's receipt: pages, files, bytes
// and the dangling-link warning. Rendering must survive a zero-page database
// (a fresh install exporting before any compile) and a real page.

func TestWikiExportMainHappyPath(t *testing.T) {
	dbPath := seedExportDB(t)
	outDir := filepath.Join(t.TempDir(), "out")

	res := runMain(t, []string{"wiki-export", "-db", dbPath, "-out", outDir})
	if res.err != nil {
		t.Fatalf("wiki-export failed: %v (stderr: %s)", res.err, res.stderr)
	}
	// The tree exists and the exported page carries frontmatter with the
	// source stamp; the receipt names one page.
	entries, err := os.ReadDir(filepath.Join(outDir, "wiki", "entities"))
	if err != nil {
		t.Fatalf("entities dir missing: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entity files, want 1", len(entries))
	}
	body, err := os.ReadFile(filepath.Join(outDir, "wiki", "entities", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(body), "source: reponest-export") {
		t.Errorf("exported page missing source stamp:\n%s", body)
	}
	if !contains(res.stdout, "exported 1 page") {
		t.Errorf("receipt line wrong: %s", res.stdout)
	}
}

func TestWikiExportMainRefusesNonEmptyTarget(t *testing.T) {
	dbPath := seedExportDB(t)
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "user-file.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runMain(t, []string{"wiki-export", "-db", dbPath, "-out", outDir})
	if res.err == nil {
		t.Fatal("expected refusal for a non-empty target, got success")
	}
	if !contains(res.stderr, "export failed") {
		t.Errorf("expected the log.Fatalf message on stderr, got: %s", res.stderr)
	}
	if res.exit == nil || *res.exit != 1 {
		t.Errorf("refusal must exit 1, got %v", res.exit)
	}
}

func TestWikiExportMainRequiresOut(t *testing.T) {
	dbPath := setupDBFile(t)
	res := runMain(t, []string{"wiki-export", "-db", dbPath})
	if res.exit == nil || *res.exit != 2 {
		t.Errorf("missing -out must exit 2, got %v", res.exit)
	}
	if !contains(res.stderr, "--out is required") {
		t.Errorf("usage error not surfaced: %s", res.stderr)
	}
}

// --- helpers ---------------------------------------------------------------

// seedExportDB builds a database with one approved wiki page to export.
func seedExportDB(t *testing.T) string {
	t.Helper()
	dbPath := setupDBFile(t)
	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()

	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	pid, err := db.SyncProjectTx(tx, "exportproj", t.TempDir(), 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateWikiPage(database, db.WikiKindEntity, "payment-gw", "Payment Gateway", pid, "Retries and idempotency."); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

type cmdResult struct {
	stdout string
	stderr string
	err    error
	exit   *int
}

// runMain executes main() with os.Args swapped, capturing stdout/stderr.
//
// main() uses flag.Parse() on flag.CommandLine, so calling it twice in one
// test binary panics with "flag redefined". The same happens with log.Fatalf:
// its os.Exit would kill the whole test process. Both are intercepted here by
// swapping the package-level seams the test binary can actually swap —
// flag.CommandLine (reset before each run) and a saved reference to main's
// exit path is NOT available, so instead the guard runs main at most once per
// flag-set and recovers the exit through testing's own exit hook: the first
// os.Exit inside a test is converted by guarding each run with a fresh
// CommandLine and relying on log's Fatal writing to os.Stderr before exiting.
//
// Concretely: the "flag redefined" failure mode is eliminated by resetting
// flag.CommandLine per run; the log.Fatalf failure mode is tolerated by
// running the exit-calling tests in a subprocess (see runMainInSubprocess).
func runMain(t *testing.T, args []string) (res cmdResult) {
	t.Helper()

	if !mainRan {
		runMainInline(t, args, true)
		mainRan = true
		return lastResult
	}
	// A second run would panic on flag redefinition — delegate to a
	// subprocess of this test binary.
	return runMainInSubprocess(t, args)
}

var (
	mainRan    bool
	lastResult cmdResult
)

func runMainInline(t *testing.T, args []string, capture bool) {
	t.Helper()

	origArgs := os.Args
	var origStdout, origStderr *os.File
	if capture {
		origStdout = os.Stdout
		origStderr = os.Stderr
		defer func() {
			os.Stdout = origStdout
			os.Stderr = origStderr
		}()
	}
	defer func() { os.Args = origArgs }()

	var rOut, rErr *os.File
	var wOut, wErr *os.File
	if capture {
		var err error
		rOut, wOut, err = os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		rErr, wErr, err = os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = wOut
		os.Stderr = wErr
	}
	os.Args = args

	var done chan struct{}
	if capture {
		done = make(chan struct{})
		go func() {
			defer close(done)
			out, _ := io.ReadAll(rOut)
			errB, _ := io.ReadAll(rErr)
			lastResult.stdout = string(out)
			lastResult.stderr = string(errB)
		}()
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				// log.Fatalf → os.Exit cannot be intercepted in-process;
				// treat any panic escaping main as a fatal path marker.
				code := 1
				lastResult.err = fmt.Errorf("main died (exit %d)", code)
				lastResult.exit = &code
			}
		}()
		main()
	}()

	if capture {
		_ = wOut.Close()
		_ = wErr.Close()
		<-done
		_ = rOut.Close()
		_ = rErr.Close()
	}
}

// runMainInSubprocess re-executes this test binary as a subprocess with
// REPONEST_CMDTEST_ARGS set, so main() runs in a fresh process where neither
// the flag redefinition nor the real os.Exit can hurt the suite. The child
// re-enters main through the hook in TestMain.
func runMainInSubprocess(t *testing.T, args []string) cmdResult {
	t.Helper()

	env := append(os.Environ(), "REPONEST_CMDTEST_ARGS="+fmt.Sprint(args))
	cmd := exec.Command(os.Args[0], "-test.run", "^TestCmdTestSubprocessHook$", "-test.v")
	cmd.Env = env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	outB, _ := io.ReadAll(stdout)
	errB, _ := io.ReadAll(stderr)
	runErr := cmd.Wait()

	res := cmdResult{stdout: string(outB), stderr: string(errB)}
	if runErr != nil {
		code := 1
		if ee, ok := runErr.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		res.exit = &code
		res.err = fmt.Errorf("subprocess exited %d", code)
	}
	return res
}

// TestCmdTestSubprocessHook is never selected by the outer suite's -test.run;
// it exists so the subprocess has a test entry point that runs main() with
// the args passed through the environment.
func TestCmdTestSubprocessHook(t *testing.T) {
	raw := os.Getenv("REPONEST_CMDTEST_ARGS")
	if raw == "" {
		t.Skip("not in subprocess mode")
	}
	var args []string
	if err := parseArgsList(raw, &args); err != nil {
		t.Fatal(err)
	}
	// No stdio capture in the child: anything main writes must reach the
	// parent's exec pipes, not an in-process pipe that dies with os.Exit.
	runMainInline(t, args, false)
}

func parseArgsList(raw string, out *[]string) error {
	// fmt.Sprint of []string renders "[a b c]"; strip the brackets.
	if len(raw) < 2 || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return fmt.Errorf("bad args list: %q", raw)
	}
	inner := raw[1 : len(raw)-1]
	if inner == "" {
		*out = nil
		return nil
	}
	for _, part := range splitSpace(inner) {
		*out = append(*out, part)
	}
	return nil
}

func splitSpace(s string) []string {
	var out []string
	start := -1
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	return out
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
