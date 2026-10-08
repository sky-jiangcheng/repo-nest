package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// The wiki layer (ADR-0014 决策 1: 先补结构，再让 LLM 写).
//
// This is the substrate the compiled-knowledge pattern needs and that
// project_notes does not provide: notes today hang off a project and reference
// nothing (no link table anywhere in the schema), so "one ingest touches 10-15
// pages" is not even expressible. W1 adds structure only — no embedding, no LLM
// call, no UI.
//
// Design decisions, and the reason for each:
//
//  1. slug is UNIQUE GLOBALLY, not per project. A per-project unique index would
//     have a NULL hole: project_id is nullable (a page may span projects), and
//     SQLite treats NULLs as distinct in unique indexes, so two global pages
//     could share a slug and wikilink resolution would become ambiguous. Global
//     slugs also match how [[wikilinks]] actually resolve.
//  2. Five kinds, not the four the ADR listed. Karpathy's pattern and the
//     nashsu/llm_wiki reference implementation both keep a queries/ area: the
//     query loop files a good answer back as a page, and ADR-0014 决策 3 promises
//     exactly that. Without a kind for it, the answer has nowhere to go and the
//     loop cannot close. This resolves the ADR's open question.
//  3. page_links is one table with indexes on BOTH ends. "Bidirectional" is the
//     requirement; two tables would be redundant, and one table with a
//     to_page_id index answers backlinks directly.
//  4. note_repositories closes the other structural gap: a note can currently
//     name a project but never the repository inside it, while the whole product
//     is about repositories.
//
// Everything here is derived structure over project_notes (still the SSOT, ADR
// 决策 5): DropWikiSchema removes it and the database is byte-for-byte what it
// was, which is what makes migration v16 safe to replay and to reverse.
var wikiSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS wiki_pages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		slug TEXT NOT NULL UNIQUE,
		title TEXT NOT NULL,
		kind TEXT NOT NULL CHECK (kind IN ('entity','concept','source','synthesis','query')),
		project_id INTEGER,
		content TEXT NOT NULL DEFAULT '',
		-- ADR-0015: review state + write provenance. status defaults to 'approved'
		-- on purpose: every page predating this migration was human-initiated
		-- (written in the app, or filed via W2's File-as-page), and defaulting to
		-- 'pending' would silently evict the user's existing knowledge from
		-- retrieval the moment the app is upgraded.
		status TEXT NOT NULL DEFAULT 'approved' CHECK (status IN ('pending','approved','rejected')),
		source TEXT NOT NULL DEFAULT 'manual',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_wiki_pages_project ON wiki_pages(project_id)`,
	`CREATE INDEX IF NOT EXISTS idx_wiki_pages_kind ON wiki_pages(kind)`,
	`CREATE INDEX IF NOT EXISTS idx_wiki_pages_status ON wiki_pages(status)`,
	`CREATE TABLE IF NOT EXISTS page_links (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_page_id INTEGER NOT NULL,
		to_page_id INTEGER NOT NULL,
		relation TEXT NOT NULL DEFAULT 'ref',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		CHECK (from_page_id <> to_page_id),
		UNIQUE (from_page_id, to_page_id, relation),
		FOREIGN KEY (from_page_id) REFERENCES wiki_pages(id) ON DELETE CASCADE,
		FOREIGN KEY (to_page_id) REFERENCES wiki_pages(id) ON DELETE CASCADE
	)`,
	// The backlink direction is the one that is impossible without this index:
	// scanning page_links for "who points at me" is the whole point of a wiki.
	`CREATE INDEX IF NOT EXISTS idx_page_links_to ON page_links(to_page_id)`,
	`CREATE INDEX IF NOT EXISTS idx_page_links_from ON page_links(from_page_id)`,
	`CREATE TABLE IF NOT EXISTS note_pages (
		note_id INTEGER NOT NULL,
		page_id INTEGER NOT NULL,
		PRIMARY KEY (note_id, page_id),
		FOREIGN KEY (note_id) REFERENCES project_notes(id) ON DELETE CASCADE,
		FOREIGN KEY (page_id) REFERENCES wiki_pages(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_note_pages_page ON note_pages(page_id)`,
	`CREATE TABLE IF NOT EXISTS note_repositories (
		note_id INTEGER NOT NULL,
		repository_id INTEGER NOT NULL,
		PRIMARY KEY (note_id, repository_id),
		FOREIGN KEY (note_id) REFERENCES project_notes(id) ON DELETE CASCADE,
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_note_repositories_repo ON note_repositories(repository_id)`,
}

// WikiKind values. Kept as constants so the CHECK constraint and the Go side
// cannot drift apart silently (the DB rejects anything else at insert time).
const (
	WikiKindEntity    = "entity"
	WikiKindConcept   = "concept"
	WikiKindSource    = "source"
	WikiKindSynthesis = "synthesis"
	WikiKindQuery     = "query"
)

// WikiPage is one page of the compiled layer.
type WikiPage struct {
	ID        int64  `json:"id"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	ProjectID int64  `json:"project_id"` // 0 when the page spans projects
	Content   string `json:"content"`
	// UpdatedAt is exported and used by the lint's staleness pass: "which of two
	// conflicting pages is the newer claim" cannot be answered without it, so the
	// field belongs on the struct rather than in a second query.
	UpdatedAt string `json:"updated_at"`
	// Status gates retrieval: only "approved" pages may answer a query
	// (ADR-0015 决策 1). Inventory paths — the export bypass, lint, the review
	// surface — deliberately still see every status, because pending pages are
	// exactly what they must show and check.
	Status string `json:"status"`
	// Source is provenance of the write, not of the idea: manual | ai |
	// wiki-compile | import:<source>. It is what makes "the compiler may only
	// touch its own pending output" a checkable property instead of a promise.
	Source string `json:"source"`
}

// scanPageFields is the single Scan counterpart for the page projection. Every
// query returning WikiPage must go through it: when columns are listed in two
// places (SELECT and Scan) they drift silently after any column is added, which
// this package already did once when updated_at went in.
func scanPageFields(rows interface{ Scan(dest ...any) error }, p *WikiPage) error {
	return rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Kind, &p.ProjectID, &p.Content,
		&p.UpdatedAt, &p.Status, &p.Source)
}

// pageColumns is the projection used by queries against wiki_pages directly.
const pageColumns = "id, slug, title, kind, COALESCE(project_id, 0), content, " +
	"COALESCE(updated_at, ''), COALESCE(status, 'approved'), COALESCE(source, 'manual')"

// pageColumnsP is the same projection for queries that alias wiki_pages as p.
const pageColumnsP = "p.id, p.slug, p.title, p.kind, COALESCE(p.project_id, 0), p.content, " +
	"COALESCE(p.updated_at, ''), COALESCE(p.status, 'approved'), COALESCE(p.source, 'manual')"

// scanPageFieldsWithRank is the rank-carrying variant, kept adjacent to
// scanPageFields so the column order remains one fact rather than two.
func scanPageFieldsWithRank(rows interface{ Scan(dest ...any) error }, p *WikiPage, rank *float64) error {
	return rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Kind, &p.ProjectID, &p.Content,
		&p.UpdatedAt, &p.Status, &p.Source, rank)
}

// WikiPageStatus values (ADR-0015 决策 1).
const (
	WikiStatusPending  = "pending"
	WikiStatusApproved = "approved"
	WikiStatusRejected = "rejected"
)

// EnsureWikiReviewColumns is migration v18 (ADR-0015 决策 1): it adds status and
// source to a wiki_pages that already exists.
//
// Why it is a separate step rather than folded into EnsureWikiSchema: that
// function's CREATE TABLE IF NOT EXISTS is a no-op against an existing table, so
// a database created before these columns existed would keep the old shape while
// still being stamped v16/v17 — the silent half-migrated state this repo has
// already been burned by once with the notes FTS index (migration v12).
//
// Both errors are tolerated because a fresh database arrives with the columns
// already present from wikiSchemaStatements, so replaying the ALTER is normal,
// not exceptional.
func EnsureWikiReviewColumns(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE wiki_pages ADD COLUMN status TEXT NOT NULL DEFAULT 'approved' CHECK (status IN ('pending','approved','rejected'))`,
		`ALTER TABLE wiki_pages ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'`,
		`CREATE INDEX IF NOT EXISTS idx_wiki_pages_status ON wiki_pages(status)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			msg := err.Error()
			if strings.Contains(msg, "duplicate column name") || isAlreadyExistsErr(err) {
				continue
			}
			return fmt.Errorf("db: ensure wiki review columns: %w", err)
		}
	}
	return nil
}

// ListWikiPagesByStatus is the review surface's query: pending pages are precisely
// the ones a human has to look at, and they must be reachable without the
// approved-only retrieval path seeing them.
func ListWikiPagesByStatus(db *sql.DB, status string, projectID int64) ([]WikiPage, error) {
	q := "SELECT " + pageColumns + " FROM wiki_pages WHERE status = ?"
	args := []any{status}
	if projectID > 0 {
		q += " AND project_id = ?"
		args = append(args, projectID)
	}
	q += " ORDER BY kind, slug"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("db: list wiki pages by status: %w", err)
	}
	defer rows.Close()
	var out []WikiPage
	for rows.Next() {
		var p WikiPage
		if err := scanPageFields(rows, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountWikiPagesByStatus feeds the review badge and the compile report's
// "what is waiting" line.
func CountWikiPagesByStatus(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query("SELECT status, COUNT(*) FROM wiki_pages GROUP BY status")
	if err != nil {
		return nil, fmt.Errorf("db: count wiki pages: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var (
			status string
			n      int
		)
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, rows.Err()
}

// SetWikiPageStatus is the review transition. It is deliberately dumb — approve
// and reject are the caller's policy, and the CHECK constraint already bounds the
// states. Note the compiler has no business calling this with "approved": landing
// a page in search is a human act (ADR-0015 决策 3).
func SetWikiPageStatus(db *sql.DB, id int64, status string) error {
	if status != WikiStatusPending && status != WikiStatusApproved && status != WikiStatusRejected {
		return fmt.Errorf("db: invalid wiki status %q", status)
	}
	if _, err := db.Exec(
		"UPDATE wiki_pages SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", status, id); err != nil {
		return fmt.Errorf("db: set wiki page status: %w", err)
	}
	return nil
}

// PageEdge is one directed link; Direction says which end the query came from.
type PageEdge struct {
	PageID   int64  `json:"page_id"`
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Relation string `json:"relation"`
	Inbound  bool   `json:"inbound"` // true = a backlink (someone points at me)
}

// ValidWikiKind reports whether kind is one of the five known page kinds.
func ValidWikiKind(kind string) bool {
	switch kind {
	case WikiKindEntity, WikiKindConcept, WikiKindSource, WikiKindSynthesis, WikiKindQuery:
		return true
	}
	return false
}

// EnsureWikiSchema creates the wiki layer. Idempotent, and exported so tests and
// the migration can share one definition (same contract as EnsureFTSIndex and
// EnsureNoteEmbedDirty).
func EnsureWikiSchema(db *sql.DB) error {
	for _, stmt := range wikiSchemaStatements {
		if _, err := db.Exec(stmt); err != nil {
			if isAlreadyExistsErr(err) {
				continue
			}
			return fmt.Errorf("db: ensure wiki schema: %w", err)
		}
	}
	return nil
}

// wikiObjects is everything this layer owns, for the reversibility check and for
// DropWikiSchema. Listing them explicitly (rather than pattern-matching
// sqlite_master) means a typo in either direction fails loudly.
var wikiObjects = []struct {
	kind, name string
}{
	{"TABLE", "note_repositories"},
	{"TABLE", "note_pages"},
	{"INDEX", "idx_note_repositories_repo"},
	{"INDEX", "idx_note_pages_page"},
	{"TABLE", "page_links"},
	{"INDEX", "idx_page_links_from"},
	{"INDEX", "idx_page_links_to"},
	{"TABLE", "wiki_pages"},
	{"INDEX", "idx_wiki_pages_kind"},
	{"INDEX", "idx_wiki_pages_project"},
}

// DropWikiSchema removes the whole derived layer, restoring the pre-v16 schema.
// It also drops the v17 page index first: an external-content FTS5 table whose
// content table vanishes underneath it is exactly the broken-index state that
// migration v12 had to repair once already for notes.
func DropWikiSchema(db *sql.DB) error {
	if err := dropWikiFTS(db); err != nil {
		return err
	}
	// Children first: page_links references wiki_pages, so dropping the parent
	// table first would leave the FK pointing at nothing.
	for _, o := range wikiObjects {
		if _, err := db.Exec("DROP " + o.kind + " IF EXISTS " + o.name); err != nil {
			return fmt.Errorf("db: drop wiki %s %s: %w", o.kind, o.name, err)
		}
	}
	return nil
}

// NormalizeWikiSlug turns a title into the canonical slug form used for lookup
// and uniqueness (lowercase, spaces and punctuation to single hyphens). Exported
// because W2/W3 will resolve [[wikilinks]] through exactly this function.
func NormalizeWikiSlug(s string) string {
	var b strings.Builder
	prevDash := true // collapse a leading separator
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r > 0x7f:
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// CreateWikiPage inserts an approved, human-authored page — the historical
// semantics, kept for callers that predate ADR-0015.
func CreateWikiPage(db *sql.DB, kind, slug, title string, projectID int64, content string) (*WikiPage, error) {
	return CreateWikiPageAs(db, kind, slug, title, projectID, content, WikiStatusApproved, "manual")
}

// CreateWikiPageAs is the full form. Status and source are explicit arguments
// because who may set them is the safety model, not a detail:
//
//   - anything the compiler writes arrives as pending / source="wiki-compile";
//   - a human filing an answer (W2) is approved at birth with source="ai",
//     because a person clicked the button;
//   - there is no path here that lets a caller "update" an approved page —
//     revisions take the pending route (ADR-0015 决策 3).
func CreateWikiPageAs(db *sql.DB, kind, slug, title string, projectID int64, content, status, source string) (*WikiPage, error) {
	if !ValidWikiKind(kind) {
		return nil, fmt.Errorf("db: invalid wiki kind %q", kind)
	}
	switch status {
	case WikiStatusPending, WikiStatusApproved, WikiStatusRejected:
	default:
		return nil, fmt.Errorf("db: invalid wiki status %q", status)
	}
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("db: wiki source is required")
	}
	if strings.TrimSpace(slug) == "" {
		return nil, fmt.Errorf("db: wiki slug is required")
	}
	var pid any
	if projectID > 0 {
		pid = projectID
	}
	res, err := db.Exec(
		`INSERT INTO wiki_pages(slug, title, kind, project_id, content, status, source) VALUES (?,?,?,?,?,?,?)`,
		slug, title, kind, pid, content, status, source)
	if err != nil {
		if isUniqueSlugErr(err) {
			return nil, fmt.Errorf("db: wiki slug %q already exists: %w", slug, ErrWikiSlugTaken)
		}
		if strings.Contains(err.Error(), "CHECK") {
			return nil, fmt.Errorf("db: rejected by the kind CHECK constraint (kind %q): %w", kind, err)
		}
		return nil, fmt.Errorf("db: create wiki page: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("db: create wiki page id: %w", err)
	}
	return GetWikiPageByID(db, id)
}

// ErrWikiSlugTaken distinguishes "another page owns this slug" from a generic
// insert failure, so callers can update-or-report instead of guessing.
var ErrWikiSlugTaken = fmt.Errorf("wiki slug already taken")

func isUniqueSlugErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: wiki_pages.slug")
}

// GetWikiPageByID loads one page. Returns sql.ErrNoRows when absent.
func GetWikiPageByID(db *sql.DB, id int64) (*WikiPage, error) {
	return scanPage(db.QueryRow("SELECT "+pageColumns+" FROM wiki_pages WHERE id = ?", id))
}

// GetWikiPageBySlug loads one page by its canonical slug.
func GetWikiPageBySlug(db *sql.DB, slug string) (*WikiPage, error) {
	return scanPage(db.QueryRow("SELECT "+pageColumns+" FROM wiki_pages WHERE slug = ?", slug))
}

func scanPage(row *sql.Row) (*WikiPage, error) {
	var p WikiPage
	if err := scanPageFields(row, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListWikiPages returns pages ordered by kind then slug; kind empty lists all.
func ListWikiPages(db *sql.DB, kind string, projectID int64) ([]WikiPage, error) {
	// Inventory by default: every status is returned, because the export bypass
	// must show pending pages for review and lint must check them. Retrieval has
	// its own approved-only path (SearchWikiPages), so "what exists" and "what may
	// answer a question" stay separate questions.
	q := "SELECT " + pageColumns + " FROM wiki_pages"
	var (
		wh []string
		qa []any
	)
	if kind != "" {
		wh = append(wh, "kind = ?")
		qa = append(qa, kind)
	}
	if projectID > 0 {
		wh = append(wh, "project_id = ?")
		qa = append(qa, projectID)
	}
	if len(wh) > 0 {
		q += " WHERE " + strings.Join(wh, " AND ")
	}
	q += " ORDER BY kind, slug"
	rows, err := db.Query(q, qa...)
	if err != nil {
		return nil, fmt.Errorf("db: list wiki pages: %w", err)
	}
	defer rows.Close()
	var out []WikiPage
	for rows.Next() {
		var p WikiPage
		if err := scanPageFields(rows, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateWikiPage writes title/content/kind. Changing kind is allowed (a page
// graduates from source to synthesis), and the CHECK constraint still guards it.
func UpdateWikiPage(db *sql.DB, id int64, title, kind, content string) error {
	if !ValidWikiKind(kind) {
		return fmt.Errorf("db: invalid wiki kind %q", kind)
	}
	_, err := db.Exec(
		`UPDATE wiki_pages SET title = ?, kind = ?, content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		title, kind, content, id)
	if err != nil {
		if strings.Contains(err.Error(), "CHECK") {
			return fmt.Errorf("db: rejected by the kind CHECK constraint (kind %q): %w", kind, err)
		}
		return fmt.Errorf("db: update wiki page: %w", err)
	}
	return nil
}

// DeleteWikiPage removes a page; its links and note attachments cascade away.
func DeleteWikiPage(db *sql.DB, id int64) error {
	if _, err := db.Exec("DELETE FROM wiki_pages WHERE id = ?", id); err != nil {
		return fmt.Errorf("db: delete wiki page: %w", err)
	}
	return nil
}

// LinkWikiPages records from -> to. Idempotent: re-linking the same triple is not
// an error and does not duplicate the edge.
func LinkWikiPages(db *sql.DB, fromID, toID int64, relation string) error {
	if fromID == toID {
		return fmt.Errorf("db: refusing a self-link on page %d", fromID)
	}
	if strings.TrimSpace(relation) == "" {
		relation = "ref"
	}
	_, err := db.Exec(
		`INSERT INTO page_links(from_page_id, to_page_id, relation) VALUES (?,?,?)
		 ON CONFLICT(from_page_id, to_page_id, relation) DO NOTHING`,
		fromID, toID, relation)
	if err != nil {
		// A missing endpoint surfaces as an FK error; callers get it verbatim so
		// "the page you linked to does not exist" is distinguishable.
		return fmt.Errorf("db: link wiki pages: %w", err)
	}
	return nil
}

// UnlinkWikiPages removes one directed edge.
func UnlinkWikiPages(db *sql.DB, fromID, toID int64, relation string) error {
	if strings.TrimSpace(relation) == "" {
		relation = "ref"
	}
	if _, err := db.Exec(
		`DELETE FROM page_links WHERE from_page_id = ? AND to_page_id = ? AND relation = ?`,
		fromID, toID, relation); err != nil {
		return fmt.Errorf("db: unlink wiki pages: %w", err)
	}
	return nil
}

// WikiEdgesFrom returns outgoing links; WikiEdgesTo returns backlinks. The two
// are separate queries because the whole value of the layer is answering the
// backlink direction, which a bare adjacency list cannot.
func WikiEdgesFrom(db *sql.DB, pageID int64) ([]PageEdge, error) {
	return wikiEdges(db, `p.id = l.to_page_id AND l.from_page_id = ?`, pageID, false)
}

// WikiEdgesTo returns pages that link TO pageID (backlinks).
func WikiEdgesTo(db *sql.DB, pageID int64) ([]PageEdge, error) {
	return wikiEdges(db, `p.id = l.from_page_id AND l.to_page_id = ?`, pageID, true)
}

func wikiEdges(db *sql.DB, where string, pageID int64, inbound bool) ([]PageEdge, error) {
	rows, err := db.Query(
		`SELECT p.id, p.slug, p.title, p.kind, l.relation
		   FROM page_links l JOIN wiki_pages p ON `+where+`
		  ORDER BY l.relation, p.slug`, pageID)
	if err != nil {
		return nil, fmt.Errorf("db: wiki edges: %w", err)
	}
	defer rows.Close()
	var out []PageEdge
	for rows.Next() {
		var e PageEdge
		if err := rows.Scan(&e.PageID, &e.Slug, &e.Title, &e.Kind, &e.Relation); err != nil {
			return nil, err
		}
		e.Inbound = inbound
		out = append(out, e)
	}
	return out, rows.Err()
}

// AttachNoteToPage / DetachNoteToPage link a raw note to a compiled page, so a
// page can always cite the notes it was compiled from (and a note can show which
// pages depend on it before it is edited or deleted).
func AttachNoteToPage(db *sql.DB, noteID, pageID int64) error {
	if _, err := db.Exec(
		`INSERT INTO note_pages(note_id, page_id) VALUES (?,?)
		 ON CONFLICT DO NOTHING`, noteID, pageID); err != nil {
		return fmt.Errorf("db: attach note to page: %w", err)
	}
	return nil
}

// DetachNoteFromPage removes one note/page association.
func DetachNoteFromPage(db *sql.DB, noteID, pageID int64) error {
	if _, err := db.Exec("DELETE FROM note_pages WHERE note_id = ? AND page_id = ?", noteID, pageID); err != nil {
		return fmt.Errorf("db: detach note from page: %w", err)
	}
	return nil
}

// PagesForNote lists the pages a note feeds (used by W4 lint: "editing this note
// affects these pages").
func PagesForNote(db *sql.DB, noteID int64) ([]WikiPage, error) {
	return pagesQuery(db,
		"SELECT "+pageColumnsP+"\n\t\t   FROM wiki_pages p JOIN note_pages np ON np.page_id = p.id"+
			"\n\t\t  WHERE np.note_id = ? ORDER BY p.kind, p.slug", noteID)
}

// NotesForPage lists the source notes behind a page.
func NotesForPage(db *sql.DB, pageID int64) ([]int64, error) {
	rows, err := db.Query("SELECT note_id FROM note_pages WHERE page_id = ? ORDER BY note_id", pageID)
	if err != nil {
		return nil, fmt.Errorf("db: notes for page: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AttachNoteToRepository associates a note with one repository inside its
// project — the association project_notes never had.
func AttachNoteToRepository(db *sql.DB, noteID, repositoryID int64) error {
	if _, err := db.Exec(
		`INSERT INTO note_repositories(note_id, repository_id) VALUES (?,?)
		 ON CONFLICT DO NOTHING`, noteID, repositoryID); err != nil {
		return fmt.Errorf("db: attach note to repository: %w", err)
	}
	return nil
}

// DetachNoteFromRepository removes one note/repository association.
func DetachNoteFromRepository(db *sql.DB, noteID, repositoryID int64) error {
	if _, err := db.Exec(
		"DELETE FROM note_repositories WHERE note_id = ? AND repository_id = ?",
		noteID, repositoryID); err != nil {
		return fmt.Errorf("db: detach note from repository: %w", err)
	}
	return nil
}

// RepositoriesForNote lists the repositories a note is about.
func RepositoriesForNote(db *sql.DB, noteID int64) ([]int64, error) {
	return idsQuery(db, "SELECT repository_id FROM note_repositories WHERE note_id = ? ORDER BY repository_id", noteID)
}

// NotesForRepository lists notes attached to one repository.
func NotesForRepository(db *sql.DB, repositoryID int64) ([]int64, error) {
	return idsQuery(db, "SELECT note_id FROM note_repositories WHERE repository_id = ? ORDER BY note_id", repositoryID)
}

func idsQuery(db *sql.DB, q string, arg int64) ([]int64, error) {
	rows, err := db.Query(q, arg)
	if err != nil {
		return nil, fmt.Errorf("db: ids query: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func pagesQuery(db *sql.DB, q string, arg int64) ([]WikiPage, error) {
	rows, err := db.Query(q, arg)
	if err != nil {
		return nil, fmt.Errorf("db: pages query: %w", err)
	}
	defer rows.Close()
	var out []WikiPage
	for rows.Next() {
		var p WikiPage
		if err := scanPageFields(rows, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// WikiPageCount reports how many pages exist per kind, for a later settings view
// and for the W4 lint pass to know whether there is anything to check.
func WikiPageCount(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query("SELECT kind, COUNT(*) FROM wiki_pages GROUP BY kind")
	if err != nil {
		return nil, fmt.Errorf("db: wiki page count: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var (
			kind string
			n    int
		)
		if err := rows.Scan(&kind, &n); err != nil {
			return nil, err
		}
		out[kind] = n
	}
	return out, rows.Err()
}
