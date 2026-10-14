// Package codebuddy implements the built-in CodeBuddy agent-memory
// KnowledgeImporter (ADR-0011). CodeBuddy (Tencent, IDEA/VS Code/JetBrains) keeps
// two distinct artifacts, and this importer covers BOTH in one pass — which is
// why it is two SourceNames, not one:
//
//  1. 会话转录：~/Library/Application Support/CodeBuddyExtension/Data/<uid|default>/
//     <IDE>/<uid>/history/<session>/<sub>/index.json + messages/<msgid>.json.
//     index.json carries `requests` (id/type/messages/state/startedAt/usage);
//     message bodies live in messages/<msgid>.json as
//     {role, message: "<json: role/content[{type,text}...]>", extra, createdAt}.
//     Every sampled first-user message carries
//     "Workspace Folder: <abs path>" inside a <user_info> block, so sessions
//     attribute per-project exactly like codex's session_meta.cwd — no config.
//  2. curated 记忆：~/.codebuddy/memery/*.md（用户记忆画像 Markdown）与
//     ~/.../CodeBuddyExtension/Data/Public/.memories/*.mdc（写入规则记忆），
//     agent-GLOBAL，挂 `openclaw_project` 同族的显式目标项目，未设则跳过。
//
// SECURITY: the importer is hard-scoped to exactly these three shapes and never
// reads the parents or siblings — CodeBuddyExtension/Data 旁有 OAuth token
// (CodeBuddyIDE_mcp_ide_oauth.json)，~/.codebuddy 旁有 plugins/（可执行）与
// skills/。会话侧只读 history/*/index.json + messages/*.json；记忆侧只读
// memery/*.md 与 Public/.memories/*.mdc（flat、non-recursive）。
//
// best-effort 纪律（ADR-0011 决策 5）：会话格式无公开契约——单文件坏掉只丢该
// 会话（log 一行），只有整棵树都不认识时才整源报错，避免"导入 0 条"伪装成
// "没有会话"。记忆侧是确定性 Markdown，与 openclaw/hermes 同口径。
package codebuddy

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"repotory/internal/core/plugin"
	"repotory/internal/db"
	"repotory/internal/importers/memsrc"
)

// SourceName is the stable identifier for CodeBuddy session transcripts.
const SourceName = "codebuddy"

// MemorySourceName is the stable identifier for CodeBuddy curated memory
// (~/.codebuddy/memery/*.md + Public/.memories/*.mdc). A SEPARATE source from
// sessions on purpose: memory files are agent-global (persona/rules, not
// per-session), so they need an explicit target project, and per ADR-0011 决策 4
// the two stay one click each in Settings → 插件 rather than one opaque knob.
const MemorySourceName = "codebuddy-memory"

// MemoryProjectConfigKey names the config value (project name or numeric id) that
// curated CodeBuddy memory attaches to — same contract as openclaw_project /
// hermes_project: unset/unknown -> skipped (0), never scattered.
const MemoryProjectConfigKey = "codebuddy_project"

const (
	// maxWorkspaceSessions bounds how many session dirs one Import() walks.
	// CodeBuddy keeps unbounded history (243 sessions on the reference machine);
	// without a cap a first-ever import walks everything. 200 is generous for a
	// personal machine and keeps one import from turning into a multi-minute scan.
	maxWorkspaceSessions = 200
	// maxMessageFileBytes bounds one messages/<id>.json read. Message bodies are
	// small; anything larger is an embedded tool-result blob. Oversized files are
	// skipped per-file (best-effort), never failing the scan.
	maxMessageFileBytes = 2 << 20 // 2 MiB
	// maxFieldRunes bounds the retained first-prompt / last-reply text (codex 口径).
	maxFieldRunes = 3000
	// titlePromptRunes bounds how much of the first prompt goes into the title.
	titlePromptRunes = 60
	// maxMemoryBytes bounds one curated memory file read.
	maxMemoryBytes = db.MaxNoteContentLen + 1
)

// workspaceFolderRe extracts the absolute workspace path out of the first-user
// message's <user_info> block. Present in every sampled session (80/80), so a
// session that loses it is skipped rather than attributed.
var workspaceFolderRe = regexp.MustCompile(`(?m)^Workspace Folder:\s*(\S+)`)

// Importer implements plugin.KnowledgeImporter for both CodeBuddy halves.
type Importer struct {
	db *sql.DB
}

// MemoryImporter implements plugin.KnowledgeImporter for curated memory files.
type MemoryImporter struct {
	db *sql.DB
}

// New creates the session importer bound to the application database.
func New(database *sql.DB) *Importer { return &Importer{db: database} }

// NewMemory creates the curated-memory importer bound to the database.
func NewMemory(database *sql.DB) *MemoryImporter { return &MemoryImporter{db: database} }

// Source returns "codebuddy".
func (i *Importer) Source() string { return SourceName }

// Probe reports whether CodeBuddy session history exists on this machine: at
// least one <uid>/<IDE>/history directory under CodeBuddyExtension/Data.
// Reuses the same sessionRoots discovery as Import and stays read-only.
func (i *Importer) Probe() plugin.ProbeResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return plugin.ProbeResult{Detail: "无法解析用户主目录（CodeBuddy 会话探活）"}
	}
	roots := sessionRoots(home)
	if len(roots) == 0 {
		return plugin.ProbeResult{Detail: "未发现 CodeBuddy 会话记录（CodeBuddyExtension/Data 下没有 history 目录）"}
	}
	return plugin.ProbeResult{Detected: true, Detail: fmt.Sprintf("CodeBuddyExtension/Data：发现 %d 个 history 目录", len(roots))}
}

// Source returns "codebuddy-memory".
func (i *MemoryImporter) Source() string { return MemorySourceName }

// Probe reports whether CodeBuddy's curated memory exists on this machine: at
// least one *.md in ~/.codebuddy/memery or *.mdc in Public/.memories. Flat
// listing of exactly those allowlisted dirs — never plugins/, skills/ or the
// OAuth token files beside them.
func (i *MemoryImporter) Probe() plugin.ProbeResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return plugin.ProbeResult{Detail: "无法解析用户主目录（CodeBuddy 记忆探活）"}
	}
	dirs := []struct {
		path string
		name string
	}{
		{filepath.Join(home, ".codebuddy", "memery"), "memery/*.md"},
		{filepath.Join(home, "Library", "Application Support", "CodeBuddyExtension", "Data", "Public", ".memories"), ".memories/*.mdc"},
	}
	total, foundParts := 0, make([]string, 0, 2)
	for _, d := range dirs {
		entries, err := os.ReadDir(d.path)
		if err != nil {
			continue
		}
		n := 0
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(e.Name(), ".md") || strings.HasSuffix(e.Name(), ".mdc") {
				n++
			}
		}
		if n > 0 {
			total += n
			foundParts = append(foundParts, fmt.Sprintf("%s ×%d", d.name, n))
		}
	}
	if total == 0 {
		return plugin.ProbeResult{Detail: "未发现 CodeBuddy 记忆文件（memery/ 与 .memories/ 下没有 *.md/*.mdc）"}
	}
	return plugin.ProbeResult{Detected: true, Detail: "发现 CodeBuddy 记忆：" + strings.Join(foundParts, "，")}
}

// sessionRoots returns the CodeBuddy history roots that exist: one per
// <uid|default>/<IDE> profile under CodeBuddyExtension/Data. Missing roots are a
// successful no-op (CodeBuddy not installed), never an error.
func sessionRoots(home string) []string {
	base := filepath.Join(home, "Library", "Application Support", "CodeBuddyExtension", "Data")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []string
	for _, uid := range entries {
		if !uid.IsDir() {
			continue // defense in depth: Data/ holds only profile dirs
		}
		ides, err := os.ReadDir(filepath.Join(base, uid.Name()))
		if err != nil {
			continue
		}
		for _, ide := range ides {
			if !ide.IsDir() {
				continue
			}
			h := filepath.Join(base, uid.Name(), ide.Name(), "history")
			if st, err := os.Stat(h); err == nil && st.IsDir() {
				out = append(out, h)
			}
		}
	}
	sort.Strings(out) // stable order so tests and scans are reproducible
	return out
}

// parsed is the minimal state extracted from one session.
type parsed struct {
	sessionID     string
	workspace     string
	startedAt     int64
	firstUser     string
	lastAssistant string
}

// Import walks every CodeBuddy history tree and yields one compact log note per
// session with usable content. A missing install is a successful no-op; each
// session dir is its own failure domain (a corrupt file drops that session, logged,
// never the scan).
func (i *Importer) Import() ([]plugin.ImportDoc, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil // cannot resolve home: no-op, not a failure
	}
	projects, _ := db.GetAllProjects(i.db)
	repos, _ := db.GetAllRepositories(i.db)

	var docs []plugin.ImportDoc
	scanned := 0
	for _, root := range sessionRoots(home) {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if scanned >= maxWorkspaceSessions {
				log.Printf("codebuddy importer: reached %d sessions, stopping (remaining history left for the next run)", maxWorkspaceSessions)
				return docs, nil
			}
			scanned++
			if doc, ok := i.docFor(filepath.Join(root, e.Name()), projects, repos); ok {
				docs = append(docs, doc)
			}
		}
	}
	return docs, nil
}

// docFor reads one <session>/ subtree and builds its ImportDoc. ok=false when the
// subtree is unreadable, the index is unparseable, the first-user message has no
// workspace folder (cannot attribute per ADR-0011), or the session has no retained
// text on either side.
func (i *Importer) docFor(dir string, projects []db.Project, repos []db.Repository) (plugin.ImportDoc, bool) {
	p, ok := parseSession(dir)
	if !ok {
		return plugin.ImportDoc{}, false
	}
	pid := int64(0)
	if p.workspace != "" {
		pid = memsrc.MatchProject(memsrc.LastPathSegment(p.workspace), projects, repos)
	}
	id8 := shortID(p.sessionID)
	title := "CodeBuddy · "
	if snip := firstLine(p.firstUser); snip != "" {
		title += snip
	} else {
		title += "会话"
	}
	title += " · " + id8
	var b strings.Builder
	fmt.Fprintf(&b, "> 由 Repotory 从 CodeBuddy 会话记录自动导入（source: codebuddy，**best-effort**：只取首条提问与末条回复，不是完整转录）\n\n")
	fmt.Fprintf(&b, "**工作目录**：`%s`\n\n", p.workspace)
	if p.firstUser != "" {
		b.WriteString("## 首条提问\n\n" + truncateRunes(p.firstUser, maxFieldRunes) + "\n\n")
	}
	if p.lastAssistant != "" {
		b.WriteString("## 末条回复\n\n" + truncateRunes(p.lastAssistant, maxFieldRunes) + "\n")
	}
	return plugin.ImportDoc{
		ProjectID: pid,
		Title:     truncateRunes(title, db.MaxNoteTitleLen),
		Content:   memsrc.ClipToBytes(b.String(), db.MaxNoteContentLen),
		Kind:      "log",
		Tags:      "codebuddy",
		Source:    SourceName,
	}, true
}

// parseSession reads <session>/<sub>/index.json and its messages/ files, returning
// the session id, workspace folder, earliest startedAt, first user text and last
// assistant text. One subdir wins per session dir: the first that parses — a
// half-written fork of the same conversation cannot double-count it.
func parseSession(dir string) (parsed, bool) {
	var out parsed

	subs, err := os.ReadDir(dir)
	if err != nil {
		return out, false
	}
	for _, sub := range subs {
		if !sub.IsDir() {
			continue
		}
		idxPath := filepath.Join(dir, sub.Name(), "index.json")
		raw, err := memsrc.ReadCapped(idxPath, maxMessageFileBytes)
		if err != nil {
			continue // unreadable variant of the session: try the next subdir
		}
		if one, ok := parseIndex(raw, filepath.Join(dir, sub.Name(), "messages")); ok {
			return one, true
		}
	}
	return out, false
}

// parseIndex extracts the session id, workspace folder and earliest startedAt from
// parseIndex extracts the session id, workspace folder and earliest startedAt from
// one index.json, and returns the first user text and last assistant text from the
// sibling messages dir.
func parseIndex(raw []byte, messagesDir string) (parsed, bool) {
	var ci codebuddyIndex
	if err := json.Unmarshal(raw, &ci); err != nil {
		return parsed{}, false
	}
	var m rawMap
	if len(ci.Requests) == 0 {
		return parsed{}, false
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return parsed{}, false
	}
	id, _ := m.getString("id")
	if id == "" {
		id = ci.Requests[0].ID
	}
	var startedAt int64
	var requests []codebuddyRequest
	for _, r := range ci.Requests {
		requests = append(requests, r)
		if r.StartedAt > 0 {
			if startedAt == 0 || r.StartedAt < startedAt {
				startedAt = r.StartedAt
			}
		}
	}
	if startedAt == 0 {
		startedAt = time.Now().Unix()
	}
	out := parsed{sessionID: id, startedAt: startedAt}
	out.workspace = parseWorkspace(raw)
	out.firstUser = firstUserText(messagesDir, requests)
	out.lastAssistant = lastAssistantText(messagesDir, requests)
	return out, true
}

// parseWorkspace finds the first user_info block carrying `Workspace Folder:`.
func parseWorkspace(raw []byte) string {
	var m rawMap
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for k, v := range m {
		if strings.HasPrefix(k, "user_info") {
			var block rawMap
			if err := json.Unmarshal(v, &block); err != nil {
				continue
			}
			var wf string
			if wfu, ok := block.getString("workspace_folder"); ok {
				if err := json.Unmarshal([]byte(wfu), &wf); err != nil {
					var b []byte
					if json.Unmarshal(v, &b) == nil {
						if ms := workspaceFolderRe.FindSubmatch(b); len(ms) == 2 {
							return strings.TrimSpace(string(ms[1]))
						}
					}
				}
				if wf != "" {
					return wf
				}
			}
		}
	}
	return ""
}

// requestStart resolves the startedAt of one message to its request, if any.
func requestStart(requests []codebuddyRequest, id string) int64 {
	for _, r := range requests {
		if r.ID == id {
			return r.StartedAt
		}
	}
	return 0
}

// lastAssistantText returns the last assistant turn by startedAt, preferring
// <answer>-tagged content; falls back to the last non-null assistant text.
func lastAssistantText(messagesDir string, requests []codebuddyRequest) string {
	if messagesDir == "" {
		return ""
	}
	type entry struct {
		startedAt int64
		role      string
		text      string
	}
	var out entry
	files, err := os.ReadDir(messagesDir)
	if err != nil {
		return ""
	}
	ids := map[string]struct{}{}
	for _, r := range requests {
		ids[r.ID] = struct{}{}
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		raw, err := memsrc.ReadCapped(filepath.Join(messagesDir, f.Name()), maxMessageFileBytes)
		if err != nil {
			continue
		}
		var m rawMap
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		role, _ := m.getString("role")
		content, _ := m.getString("content")
		if role != "assistant" || content == "" {
			continue
		}
		id, _ := m.getString("id")
		if id == "" {
			continue
		}
		if _, ok := ids[id]; !ok {
			continue
		}
		startedAt := requestStart(requests, id)
		if isAnswer(m["message"]) {
			if startedAt > out.startedAt {
				out = entry{startedAt: startedAt, role: "assistant", text: content}
			}
			continue
		}
		if startedAt >= out.startedAt {
			out = entry{startedAt: startedAt, role: "assistant", text: content}
		}
	}
	return textOf(out.text)
}

// isAnswer reports whether the message field carries the internal <answer> tag.
func isAnswer(raw json.RawMessage) bool {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return false
	}
	return strings.Contains(s, "<answer>")
}

func firstUserText(messagesDir string, requests []codebuddyRequest) string {
	if messagesDir == "" {
		return ""
	}
	seen := map[string]bool{}
	files, err := os.ReadDir(messagesDir)
	if err != nil {
		return ""
	}
	type msg struct {
		requestID string
		role      string
		text      string
	}
	var msgs []msg
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		raw, err := memsrc.ReadCapped(filepath.Join(messagesDir, f.Name()), maxMessageFileBytes)
		if err != nil {
			continue
		}
		var m rawMap
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		role, _ := m.getString("role")
		content, _ := m.getString("content")
		if role != "user" || content == "" {
			continue
		}
		id, _ := m.getString("id")
		if id == "" {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		msgs = append(msgs, msg{requestID: id, role: role, text: content})
	}
	sort.Slice(msgs, func(i, j int) bool {
		if msgs[i].requestID == msgs[j].requestID {
			return false
		}
		return msgs[i].requestID < msgs[j].requestID
	})
	for _, m := range msgs {
		if m.role == "user" {
			return textOf(m.text)
		}
	}
	return ""
}

// codebuddyIndex mirrors the CodeBuddy index.json surface relevant to import.
type codebuddyIndex struct {
	Requests []codebuddyRequest `json:"requests"`
}

type rawMap map[string]json.RawMessage

// getString extracts a JSON string value for a key.
func (m rawMap) getString(key string) (string, bool) {
	if m == nil {
		return "", false
	}
	var s string
	if err := json.Unmarshal(m[key], &s); err != nil {
		return "", false
	}
	return s, true
}

type codebuddyRequest struct {
	ID        string   `json:"id"`
	StartedAt int64    `json:"startedAt"`
	Messages  []string `json:"messages"`
}

// MemoryImporter implements the curated-side import. Files are deterministic
// Markdown and must land under one explicit project; a missing project means a
// successful no-op, never partial writes.
func (m *MemoryImporter) Import() ([]plugin.ImportDoc, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	projects, _ := db.GetAllProjects(m.db)
	repos, _ := db.GetAllRepositories(m.db)
	cfg, _ := db.GetConfig(m.db, MemoryProjectConfigKey)

	var docs []plugin.ImportDoc
	pid := memsrc.MatchProject(memsrc.LastPathSegment(cfg), projects, repos)
	if pid == 0 {
		// No target configured: curated memory is agent-global and intentionally
		// not scattered into the first project the importer guesses.
		return docs, nil
	}

	if err := m.addMemery(home, pid, &docs); err != nil {
		log.Printf("codebuddy-memory importer: memery read failed: %v", err)
	}
	if err := m.addPublic(home, pid, &docs); err != nil {
		log.Printf("codebuddy-memory importer: public memory read failed: %v", err)
	}
	return docs, nil
}

// addMemery walks ~/.codebuddy/memery/*.md and turns every file into a note.
func (m *MemoryImporter) addMemery(home string, pid int64, docs *[]plugin.ImportDoc) error {
	root := filepath.Join(home, ".codebuddy", "memery")
	return m.addFiles(root, pid, docs)
}

// addPublic walks Public/.memories/*.mdc and turns every file into a note.
func (m *MemoryImporter) addPublic(home string, pid int64, docs *[]plugin.ImportDoc) error {
	base := filepath.Join(home, "Library", "Application Support", "CodeBuddyExtension", "Data", "Public", ".memories")
	return m.addFiles(base, pid, docs)
}

// addFiles imports every .md/.mdc file under root into docs. Unknown or empty
// files are per-file best-effort skips, never aborting the scan.
func (m *MemoryImporter) addFiles(root string, pid int64, docs *[]plugin.ImportDoc) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil // silently no-op when the curated dir does not exist yet
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") && !strings.HasSuffix(e.Name(), ".mdc") {
			continue
		}
		path := filepath.Join(root, e.Name())
		raw, err := memsrc.ReadCapped(path, maxMemoryBytes)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(raw))
		if content == "" {
			continue
		}
		*docs = append(*docs, plugin.ImportDoc{
			ProjectID: pid,
			Title:     truncateRunes(fallbackTitle(e.Name()), db.MaxNoteTitleLen),
			Content:   memsrc.ClipToBytes(content, db.MaxNoteContentLen),
			Kind:      "knowledge",
			Tags:      "codebuddy",
			Source:    MemorySourceName,
		})
	}
	return nil
}

// fallbackTitle keeps a memory-friendly title when the English filename has no
// useful stem.
func fallbackTitle(name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		return "CodeBuddy 记忆"
	}
	return stem
}

// shortID turns a long session id into a short stable token.
func shortID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// firstLine keeps only the first line of a message body for title extraction.
func firstLine(s string) string {
	idx := strings.IndexByte(s, '\n')
	if idx < 0 {
		return s
	}
	return s[:idx]
}

// truncateRunes keeps at most n runes of s, preserving validity.
func truncateRunes(s string, n int) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= n {
			break
		}
		b.WriteRune(r)
	}
	if b.Len() == len(s) {
		return s
	}
	return b.String()
}

// textOf extracts the leading textual content from a rich message payload, dropping
// tool results and resource blocks.
func textOf(s string) string {
	var doc struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		return s
	}
	if doc.Role != "user" {
		return s
	}
	var parts []string
	for _, c := range doc.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}
