// Package cursor implements the built-in Cursor session KnowledgeImporter
// (ADR-0011 framework, sixth source after claude / codex / opencode / openclaw /
// hermes).
//
// Cursor keeps its state in ONE SQLite database under its globalStorage dir:
//
//	<config>/Cursor/User/globalStorage/state.vscdb
//	  composerHeaders(composerId, workspaceId, createdAt, lastUpdatedAt,
//	                    isArchived, isSubagent, recency, ...)  one row per session
//	  cursorDiskKV(key, value)
//	    composerData:<composerId>          session envelope JSON (metadata, lists)
//	    bubbleId:<composerId>:<bubbleId>   one message, JSON with type/text/createdAt
//
//	<config>/Cursor/User/workspaceStorage/<32-hex>/workspace.json  {"folder":"file:///path"}
//
// Verified against a real install (macOS, 7.1 MB, 71 bubbles): bubble `text` is
// never null, `createdAt` is an ISO-8601 string, `type` is Cursor's numeric
// sender enum (1 = user, 2 = assistant), and `composerHeaders.workspaceId` is the
// same 32-hex name as the workspaceStorage directory — which makes the
// workspaceId → folder → RepoNest project chain resolvable instead of guessed.
//
// BEST EFFORT, ON PURPOSE. This format is undocumented and versioned (every JSON
// payload carries a `_v` field), so it drifts. Two rules follow from that and
// both are load-bearing:
//
//  1. Recognizable-but-unparseable rows are skipped individually (a real bubble
//     in the sample above has malformed JSON — `json_extract` errors on it).
//  2. A missing table or column makes the WHOLE SOURCE fail loudly rather than
//     import zero notes quietly. An "ok, 0 created" result on a drifted format
//     would look identical to "the user has no Cursor sessions", which is the
//     failure mode worth refusing.
//
// Registered as an OPT-IN manual source (like codex / opencode): raw session
// transcripts are sensitive, so this never runs at startup. See ADR-0011 决策 4.
package cursor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"repo-nest/internal/core/plugin"
	"repo-nest/internal/db"
	"repo-nest/internal/importers/memsrc"
)

// SourceName is the stable knowledge-source identifier for Cursor sessions.
const SourceName = "cursor"

const (
	// maxComposers bounds one import run's work. Sessions are ordered
	// most-recently-updated first, so the cap drops old history rather than the
	// context the user is currently working in.
	maxComposers = 50
	// maxBubbleScan limits how many messages of one composer we read while
	// looking for the first prompt and the last reply. A 500-bubble session is
	// not worth walking entirely for a two-excerpt note.
	maxBubbleScan = 200
	// excerptChars caps each rendered excerpt. Bubble text can carry a whole
	// file diff; the note is a pointer to the session, not a copy of it.
	excerptChars = 3000
	// stateFileLimit guards the fallback JSON path (reading composerData from the
	// KV table) — the same "never load an unexpected multi-GB blob" rule the
	// other importers apply via memsrc.ReadCapped.
	stateFileLimit = 4 << 20
)

// Importer implements plugin.KnowledgeImporter for Cursor sessions.
type Importer struct {
	db *sql.DB
}

// New creates a Cursor importer bound to the application database.
func New(database *sql.DB) *Importer { return &Importer{db: database} }

// Source returns the stable source identifier "cursor".
func (i *Importer) Source() string { return SourceName }

// composer is one Cursor session header row.
type composer struct {
	ComposerID  string
	WorkspaceID string
	CreatedAt   sql.NullInt64
	UpdatedAt   sql.NullInt64
}

// bubble is one message. Type uses Cursor's sender enum; Text is already
// truncated server-side (SQL substr) so a giant diff never reaches memory.
type bubble struct {
	Type      int
	Text      string
	CreatedAt string
}

// Import walks Cursor's state database and returns one note per session that can
// be attributed to a known RepoNest project.
//
// Ordering constraint, not style: the read-only handle allows ONE connection, so
// composerHeaders must be fully collected and its rows closed before any per-
// composer bubble query runs. Doing them concurrently deadlocks (confirmed the
// hard way by a test that did exactly that).
//
// Returns (nil, nil) when Cursor is simply not installed (nothing to do) and a
// non-nil error when it IS installed but the layout is not recognized (drift):
// the two must not look the same to the caller.
func (i *Importer) Import() ([]plugin.ImportDoc, error) {
	statePath, ok := stateDBPath()
	if !ok {
		return nil, nil
	}
	if _, err := os.Stat(statePath); err != nil {
		return nil, nil // no state database: Cursor absent, a successful no-op
	}

	st, err := openReadOnly(statePath)
	if err != nil {
		return nil, fmt.Errorf("cursor importer: open %s: %w", statePath, err)
	}
	defer st.close()

	if err := probeLayout(st.sql); err != nil {
		// Loud on purpose: see the package comment. Importing nothing here would
		// be indistinguishable from "you have no sessions".
		return nil, fmt.Errorf("cursor importer: %w", err)
	}

	projects, _ := db.GetAllProjects(i.db)
	repos, _ := db.GetAllRepositories(i.db)
	wsProjects := workspaceProjects(statePath, projects, repos)

	rows, err := st.sql.Query(
		`SELECT composerId, COALESCE(workspaceId, ''), createdAt, lastUpdatedAt
		   FROM composerHeaders
		  WHERE COALESCE(isSubagent, 0) = 0
		    AND COALESCE(workspaceId, '') NOT IN ('', 'empty-window')
		  ORDER BY COALESCE(lastUpdatedAt, createdAt) DESC
		  LIMIT ?`, maxComposers)
	if err != nil {
		return nil, fmt.Errorf("cursor importer: read composerHeaders: %w", err)
	}
	var composers []composer
	for rows.Next() {
		var (
			c                composer
			created, updated sql.NullInt64
		)
		if err := rows.Scan(&c.ComposerID, &c.WorkspaceID, &created, &updated); err != nil {
			rows.Close() //nolint:errcheck // best effort after a scan failure
			return nil, fmt.Errorf("cursor importer: scan composerHeaders: %w", err)
		}
		c.CreatedAt, c.UpdatedAt = created, updated
		composers = append(composers, c)
	}
	rows.Close() //nolint:errcheck // read finished; the error is not actionable
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cursor importer: iterate composerHeaders: %w", err)
	}

	var docs []plugin.ImportDoc
	skippedNoProject, skippedNoText := 0, 0
	for _, c := range composers {
		ws, mapped := wsProjects[c.WorkspaceID]
		if !mapped {
			// Every entry in the map already resolved to a real project, so an
			// absent key covers both "no workspace.json" and "no matching
			// project": the session is skipped rather than guessed onto one.
			skippedNoProject++
			continue
		}
		first, last, ok := i.excerpts(st.sql, c.ComposerID)
		if !ok {
			skippedNoText++
			continue
		}
		docs = append(docs, docFor(c, ws.path, ws.pid, first, last))
	}
	if len(docs) == 0 && len(composers) > 0 {
		log.Printf("cursor importer: %d session(s) found, %d skipped for no project match, %d for no usable message",
			len(composers), skippedNoProject, skippedNoText)
	}
	return docs, nil
}

// excerpts returns the first user prompt and the last assistant reply of one
// composer. Ordering uses each bubble's own ISO-8601 createdAt — the bubble keys
// embed random UUIDs, so they cannot be ordered lexically.
func (i *Importer) excerpts(st *sql.DB, composerID string) (first, last string, ok bool) {
	// Escape LIKE wildcards: a composer id is a UUID in practice, but this is
	// attacker-influenced data (another tool's database), not our own.
	pattern := "bubbleId:" + escapeLike(composerID) + ":%"
	rows, err := st.Query(
		`SELECT COALESCE(CAST(json_extract(value, '$.type') AS INTEGER), 0),
		        COALESCE(json_extract(value, '$.createdAt'), ''),
		        COALESCE(substr(json_extract(value, '$.text'), 1, ?), '')
		   FROM cursorDiskKV
		  WHERE key LIKE ? ESCAPE '\'
		  LIMIT ?`, excerptChars, pattern, maxBubbleScan)
	if err != nil {
		return "", "", false
	}
	defer rows.Close()

	// A malformed JSON value yields NULLs from json_extract; COALESCE turns that
	// into type 0 / empty text, so the row is simply unusable rather than an
	// error. The sample database contains exactly one such row.
	var bubbles []bubble
	for rows.Next() {
		var b bubble
		if err := rows.Scan(&b.Type, &b.CreatedAt, &b.Text); err != nil {
			return "", "", false
		}
		if strings.TrimSpace(b.Text) == "" {
			continue
		}
		bubbles = append(bubbles, b)
	}
	if len(bubbles) == 0 {
		return "", "", false
	}
	// ISO-8601 sorts lexically, but only when every value parses; fall back to
	// table order for anything odd rather than dropping the session.
	sortByCreatedAt(bubbles)

	for _, b := range bubbles {
		if b.Type == senderUser && first == "" {
			first = b.Text
		}
	}
	for _, b := range bubbles {
		if b.Type == senderAssistant {
			last = b.Text // keep walking: the last one wins
		}
	}
	// A session with only one side is still worth a note (an interrupted
	// question is real history), so we need at least one of the two.
	if first == "" && last == "" {
		return "", "", false
	}
	return first, last, true
}

// state is the read-only handle on Cursor's own database. It is deliberately a
// separate type: the only thing this package may ever do to that file is read it
// (ADR-0011 决策 3 — importers never mutate another tool's data).
type state struct {
	sql *sql.DB
}

func (s *state) close() { _ = s.sql.Close() }

// openReadOnly opens another application's live SQLite file without writing to
// it. Two DSN variants because Cursor runs in WAL mode: plain `mode=ro` reads
// the WAL correctly but fails when the sidecar `-shm` cannot be created or is
// owned by another user, and `immutable=1` skips WAL handling entirely at the
// cost of possibly reading a slightly stale snapshot. Preferring the accurate
// read and falling back to the tolerant one gets a useful import from a running
// Cursor without ever risking its data.
func openReadOnly(path string) (*state, error) {
	dsns := []string{
		"file:" + filepath.ToSlash(path) + "?mode=ro",
		"file:" + filepath.ToSlash(path) + "?mode=ro&immutable=1",
	}
	var lastErr error
	for _, dsn := range dsns {
		st, err := sql.Open("sqlite", dsn)
		if err != nil {
			lastErr = err
			continue
		}
		// sql.Open is lazy; only a ping proves the file is readable as SQLite.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = st.PingContext(ctx)
		cancel()
		if err != nil {
			_ = st.Close()
			lastErr = err
			continue
		}
		// One connection: this is a read of another app's database, and SQLite
		// serializes writers anyway.
		st.SetMaxOpenConns(1)
		return &state{sql: st}, nil
	}
	return nil, lastErr
}

// requiredTables / requiredColumns are the layout this importer reads. Anything
// missing means Cursor's format moved under us, which must be reported instead of
// silently importing zero notes.
var (
	requiredTables = []string{"composerHeaders", "cursorDiskKV"}
	requiredCols   = []string{"composerId", "workspaceId", "createdAt", "lastUpdatedAt", "isSubagent"}
)

func probeLayout(st *sql.DB) error {
	for _, t := range requiredTables {
		var n int
		if err := st.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?", t).Scan(&n); err != nil {
			return fmt.Errorf("probe %s: %w", t, err)
		}
		if n == 0 {
			return fmt.Errorf("unrecognized Cursor state layout: table %q is missing (best-effort source refusing to guess; nothing was imported)", t)
		}
	}
	have := map[string]bool{}
	rows, err := st.Query("PRAGMA table_info(composerHeaders)")
	if err != nil {
		return fmt.Errorf("probe composerHeaders columns: %w", err)
	}
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close() //nolint:errcheck // probe already failing, error reported below
			break
		}
		have[name] = true
	}
	rows.Close() //nolint:errcheck // probe result already captured in `have`
	for _, c := range requiredCols {
		if !have[c] {
			return fmt.Errorf("unrecognized Cursor state layout: composerHeaders.%s is missing (best-effort source refusing to guess; nothing was imported)", c)
		}
	}
	return nil
}

// sortByCreatedAt orders bubbles oldest-first using their ISO-8601 strings, which
// sort lexically. Values that do not parse keep their relative position: a
// partially-ordered transcript still yields a usable first prompt and last reply,
// which beats dropping the session because one timestamp was odd.
func sortByCreatedAt(bs []bubble) {
	var fallback []bubble
	var keyed []bubble
	for _, b := range bs {
		if _, err := time.Parse(time.RFC3339Nano, b.CreatedAt); err != nil {
			fallback = append(fallback, b)
			continue
		}
		keyed = append(keyed, b)
	}
	sort.SliceStable(keyed, func(a, c int) bool { return keyed[a].CreatedAt < keyed[c].CreatedAt })
	if len(fallback) > 0 && len(keyed) == 0 {
		return // nothing to reorder; keep table order
	}
	if len(fallback) > 0 {
		// Unparseable timestamps are treated as oldest: they came from an earlier
		// Cursor version, so surfacing the newest well-formed reply is right.
		keyed = append(append([]bubble{}, fallback...), keyed...)
	}
	copy(bs, keyed)
}

// Cursor's sender enum, as observed in a real state database (type values were
// exactly 1 and 2 across 71 bubbles). Anything else is treated as "some other
// part of the transcript" — kept out of the excerpts, never an error, because
// guessing at an undocumented enum is how a quiet corruption starts.
const (
	senderUser      = 1
	senderAssistant = 2
)

// docFor shapes one session into an import document.
func docFor(c composer, wsPath string, pid int64, first, last string) plugin.ImportDoc {
	content := memsrc.ClipToBytes(renderNote(c, wsPath, first, last), db.MaxNoteContentLen)
	return plugin.ImportDoc{
		ProjectID: pid,
		Title:     noteTitle(c.ComposerID),
		Content:   content,
		Kind:      "log",
		Tags:      "cursor",
		Source:    SourceName,
	}
}

func renderNote(c composer, wsPath, first, last string) string {
	var b strings.Builder
	b.WriteString("> 由 RepoNest 从 Cursor 会话记录自动导入（source: cursor，**best-effort**：格式未公开，只取首条提问与末条回复，不是完整转录）\n\n")
	if wsPath != "" {
		b.WriteString("**工作区**：`" + wsPath + "`\n\n")
	}
	if d := formatCursorMillis(c.CreatedAt); d != "" {
		b.WriteString("**开始**：" + d + "\n\n")
	}
	if d := formatCursorMillis(c.UpdatedAt); d != "" && d != formatCursorMillis(c.CreatedAt) {
		b.WriteString("**最后更新**：" + d + "\n\n")
	}
	if first != "" {
		b.WriteString("## 首条提问\n\n")
		b.WriteString(truncateRunes(strings.TrimSpace(first), excerptChars))
		b.WriteString("\n\n")
	}
	if last != "" {
		b.WriteString("## 末条回复\n\n")
		b.WriteString(truncateRunes(strings.TrimSpace(last), excerptChars))
		b.WriteString("\n")
	}
	b.WriteString("\n_Cursor composerId: `" + shortID(c.ComposerID) + "`_")
	return strings.TrimRight(b.String(), "\n")
}

// noteTitle keeps a stable, unique title: the runtime upserts on
// (project, source, title), so the composer id tail is what prevents two sessions
// from overwriting each other.
func noteTitle(composerID string) string {
	return "Cursor 会话 · " + shortID(composerID)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[len(id)-8:]
	}
	if id == "" {
		return "unknown"
	}
	return id
}

// stateDBPath returns Cursor's globalStorage state database path. CURSOR_STATE_DB
// overrides it (tests, non-standard installs, multi-profile setups).
func stateDBPath() (string, bool) {
	if custom := os.Getenv("CURSOR_STATE_DB"); custom != "" {
		return custom, true
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", false
	}
	// os.UserConfigDir is the one call that is right on all three platforms:
	// ~/Library/Application Support (darwin), ~/.config (linux), %APPDATA% (windows).
	return filepath.Join(configDir, "Cursor", "User", "globalStorage", "state.vscdb"), true
}

// stateRoot derives Cursor's User dir from the state database path so
// workspaceStorage lookups follow whatever root was actually used (including the
// CURSOR_STATE_DB override, which is how tests fabricate a whole install).
func stateRoot(statePath string) string {
	return filepath.Dir(filepath.Dir(statePath)) // .../User/globalStorage/state.vscdb -> .../User
}

// workspaceTarget is a resolved Cursor workspace: which RepoNest project it
// belongs to, and the folder path to print in the note.
type workspaceTarget struct {
	pid  int64
	path string
}

// workspaceProjects maps workspaceId -> (project id, folder path) by reading
// Cursor's per-workspace storage, the same layout VS Code uses. Unresolvable
// ids are simply absent from the map, and their sessions get skipped — the
// importer never attaches a session to a project it is not sure about.
func workspaceProjects(statePath string, projects []db.Project, repos []db.Repository) map[string]workspaceTarget {
	out := map[string]workspaceTarget{}
	root := filepath.Join(stateRoot(statePath), "workspaceStorage")
	entries, err := os.ReadDir(root)
	if err != nil {
		return out // no workspace storage: every session skips, which is correct
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wj := filepath.Join(root, e.Name(), "workspace.json")
		raw, err := memsrc.ReadCapped(wj, 64<<10)
		if err != nil {
			continue
		}
		path, ok := folderPathFromWorkspaceJSON(raw)
		if !ok {
			continue
		}
		pid := memsrc.MatchProject(memsrc.LastPathSegment(path), projects, repos)
		if pid == 0 {
			continue
		}
		out[e.Name()] = workspaceTarget{pid: pid, path: path}
	}
	return out
}

// folderPathFromWorkspaceJSON extracts the folder path from a workspace.json,
// accepting both a raw path and a file:// URI (Cursor writes the URI form).
func folderPathFromWorkspaceJSON(raw []byte) (string, bool) {
	var doc struct {
		Folder string `json:"folder"`
	}
	if json.Unmarshal(raw, &doc) != nil || strings.TrimSpace(doc.Folder) == "" {
		return "", false
	}
	f := doc.Folder
	if strings.HasPrefix(f, "file://") {
		u, err := url.Parse(f)
		if err != nil || u.Path == "" {
			return "", false
		}
		f = u.Path
		// Windows file URIs arrive as file:///C:/dir — drop the leading slash.
		if len(f) > 2 && f[0] == '/' && f[2] == ':' {
			f = f[1:]
		}
	}
	return filepath.Clean(f), true
}

// formatCursorMillis renders Cursor's epoch-millisecond headers as a date.
func formatCursorMillis(v sql.NullInt64) string {
	if !v.Valid || v.Int64 <= 0 {
		return ""
	}
	return time.UnixMilli(v.Int64).Format("2006-01-02")
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
