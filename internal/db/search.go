package db

import (
	"database/sql"
	"html"
	"strings"
	"unicode/utf8"
)

const (
	searchResultLimit   = 20
	searchSnippetWindow = 100
)

// SearchNotes searches note title and content, returning ranked hits. It uses
// the FTS5 trigram index (issue #18) for relevance-ranked substring matching
// when every query term is at least 3 characters; otherwise it falls back to a
// LIKE scan (which also covers 2-character CJK queries the trigram tokenizer
// cannot match). If the FTS index is unavailable or yields no rows, it falls
// back to LIKE so search never regresses.
func SearchNotes(db *sql.DB, query string) ([]SearchHit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if ftsUsable(q) {
		hits, err := searchNotesFTS(db, q, escapeFTS(q))
		if err == nil {
			if len(hits) > 0 {
				return hits, nil // strict AND matched: behaviour unchanged
			}
			// M3-C (ADR-0012): strict AND yielded ZERO rows — relax by dropping
			// English stopwords and OR-ing the remaining terms. This only ever
			// adds results in cases that returned empty before, so it cannot
			// degrade a currently-good result set (zero dictionary, zero CGO).
			// It stays WITHIN FTS (no new LIKE fallback), so a broken/empty
			// index still yields nothing exactly as before.
			if orExpr := escapeFTSOR(q); orExpr != "" {
				if relaxed, rerr := searchNotesFTS(db, q, orExpr); rerr == nil {
					return relaxed, nil
				}
			}
			return hits, nil // empty AND result: preserve original behaviour
		}
		// FTS index missing or query rejected: fall through to LIKE.
	}
	return searchNotesLike(db, q)
}

// searchNotesFTS matches a prepared FTS5 expression against the index, ranked
// by bm25. `matchExpr` is produced by escapeFTS (strict AND) or escapeFTSOR
// (relaxed); `q` is the original query, used only to render the snippet.
func searchNotesFTS(db *sql.DB, q, matchExpr string) ([]SearchHit, error) {
	rows, err := db.Query(
		"SELECT n.id, n.project_id, n.title, n.content, bm25(project_notes_fts) "+
			"FROM project_notes_fts f "+
			"JOIN project_notes n ON n.id = f.rowid "+
			"WHERE project_notes_fts MATCH ? "+
			"ORDER BY bm25(project_notes_fts), n.pinned DESC, n.updated_at DESC LIMIT ?",
		matchExpr, searchResultLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		var content string
		var rank float64
		if err := rows.Scan(&h.ID, &h.ProjectID, &h.Title, &content, &rank); err != nil {
			return nil, err
		}
		h.Type = "note"
		h.Rank = rank
		h.Snippet = makeSnippet(content, q)
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// searchNotesLike is the LIKE-based fallback used for short or special queries.
// It assigns a synthetic rank: title matches score higher than content-only
// matches, pinned notes get a bonus, and recent updates rank higher.
func searchNotesLike(db *sql.DB, q string) ([]SearchHit, error) {
	pattern := "%" + escapeLike(q) + "%"
	// Weighted rank: title match = -10, content match = -5, pinned = -20,
	// recency bonus via julian day diff (lower is better).
	// COALESCE(pinned, 0) handles the NULL case that occurs during migration
	// before the ALTER TABLE ADD COLUMN DEFAULT 0 is applied to all rows.
	rankExpr := "CASE WHEN title LIKE ? ESCAPE '\\' THEN -10 ELSE -5 END " +
		"+ CASE WHEN COALESCE(pinned, 0) = 1 THEN -20 ELSE 0 END " +
		"+ (julianday('now') - julianday(updated_at)) * -0.01"
	rows, err := db.Query(
		"SELECT id, project_id, title, content, "+rankExpr+" AS rank "+
			"FROM project_notes WHERE title LIKE ? ESCAPE '\\' OR content LIKE ? ESCAPE '\\' "+
			"ORDER BY rank LIMIT ?",
		pattern, pattern, pattern, searchResultLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		var content string
		if err := rows.Scan(&h.ID, &h.ProjectID, &h.Title, &content, &h.Rank); err != nil {
			return nil, err
		}
		h.Type = "note"
		h.Snippet = makeSnippet(content, q)
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// SearchAll searches notes and todos, returning unified ranked hits. Notes use
// the FTS5 path (with LIKE fallback) via SearchNotes; todos use the FTS index
// when usable and fall back to LIKE otherwise.
func SearchAll(db *sql.DB, query string) ([]SearchHit, error) {
	hits, err := SearchNotes(db, query)
	if err != nil {
		return nil, err
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return hits, nil
	}
	todoHits, err := searchTodos(db, q)
	if err != nil {
		return nil, err
	}
	return append(hits, todoHits...), nil
}

// searchTodos searches todo titles via FTS5 when usable, else LIKE.
func searchTodos(db *sql.DB, q string) ([]SearchHit, error) {
	if ftsUsable(q) {
		hits, err := searchTodosFTS(db, q)
		if err == nil {
			return hits, nil
		}
	}
	return searchTodosLike(db, q)
}

func searchTodosFTS(db *sql.DB, q string) ([]SearchHit, error) {
	rows, err := db.Query(
		"SELECT t.id, t.project_id, t.title FROM project_todos_fts f "+
			"JOIN project_todos t ON t.id = f.rowid "+
			"WHERE project_todos_fts MATCH ? "+
			"ORDER BY bm25(project_todos_fts), t.sort_order ASC, t.id ASC LIMIT ?",
		escapeFTS(q), searchResultLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ID, &h.ProjectID, &h.Title); err != nil {
			return nil, err
		}
		h.Type = "todo"
		h.Snippet = h.Title
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func searchTodosLike(db *sql.DB, q string) ([]SearchHit, error) {
	pattern := "%" + escapeLike(q) + "%"
	rows, err := db.Query(
		"SELECT id, project_id, title FROM project_todos WHERE title LIKE ? ESCAPE '\\' ORDER BY sort_order ASC, id ASC LIMIT ?",
		pattern, searchResultLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ID, &h.ProjectID, &h.Title); err != nil {
			return nil, err
		}
		h.Type = "todo"
		h.Snippet = h.Title
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// ftsUsable reports whether the query can be served by the trigram FTS index.
// The trigram tokenizer extracts 3-character sequences, so a term shorter than
// 3 characters cannot match; in that case the caller falls back to LIKE.
func ftsUsable(query string) bool {
	for _, term := range strings.Fields(query) {
		if utf8.RuneCountInString(term) < 3 {
			return false
		}
	}
	return len(strings.Fields(query)) > 0
}

// escapeFTS builds an FTS5 MATCH expression from the query: each
// whitespace-separated term is wrapped as a phrase (double quotes escaped) and
// terms are combined with implicit AND. Wrapping as a phrase neutralises FTS
// special characters (*, :, AND, parentheses, etc.) so user input is treated
// as literal substring terms, not query syntax.
func escapeFTS(query string) string {
	var parts []string
	for _, term := range strings.Fields(query) {
		term = strings.ReplaceAll(term, `"`, `""`)
		parts = append(parts, `"`+term+`"`)
	}
	return strings.Join(parts, " ")
}

// escapeFTSOR builds a relaxed FTS5 expression: English stopwords are dropped
// and the remaining (>=3-char, since ftsUsable gated it) terms are OR-combined,
// each quoted as a phrase. Returns "" when fewer than two non-stopword terms
// remain (a single term's OR is identical to its AND, so relaxation is pointless
// and would only risk noise).
func escapeFTSOR(query string) string {
	var parts []string
	for _, term := range strings.Fields(query) {
		if searchStopwords[strings.ToLower(term)] {
			continue
		}
		term = strings.ReplaceAll(term, `"`, `""`)
		parts = append(parts, `"`+term+`"`)
	}
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts, " OR ")
}

// searchStopwords is a tiny, deterministic English function-word set used ONLY
// for M3-C query relaxation (never to reject a strict AND match, only to avoid
// OR-ing pure noise terms). Deliberately small and not a full thesaurus — the
// goal is recall, not semantics.
var searchStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true,
	"this": true, "from": true, "they": true, "have": true, "were": true,
	"what": true, "when": true, "your": true, "about": true, "into": true,
	"there": true, "these": true, "some": true, "then": true, "than": true,
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func makeSnippet(content, query string) string {
	// HTML-escape the content first so any HTML in note text is neutralised.
	escaped := html.EscapeString(content)
	// HTML-escape the query for safe insertion into <mark> tags.
	escQuery := html.EscapeString(query)

	if len(escaped) <= searchSnippetWindow {
		return highlightQuery(escaped, escQuery)
	}
	idx := strings.Index(strings.ToLower(escaped), strings.ToLower(escQuery))
	if idx < 0 {
		return highlightQuery(escaped[:clampToRuneStart(escaped, searchSnippetWindow)], escQuery)
	}
	start := idx - searchSnippetWindow/2
	if start < 0 {
		start = 0
	}
	end := start + searchSnippetWindow
	if end > len(escaped) {
		end = len(escaped)
	}
	// Clamp both ends to valid UTF-8 boundaries so we don't split a
	// multi-byte character (common with CJK text where each rune is 3 bytes).
	// The `end < len(escaped)` guard is essential: when end == len(escaped)
	// the slice already ends on a valid boundary and escaped[end] would panic.
	for end > start && end < len(escaped) && !utf8.RuneStart(escaped[end]) {
		end--
	}
	for start < end && !utf8.RuneStart(escaped[start]) {
		start++
	}
	snippet := escaped[start:end]
	return highlightQuery(snippet, escQuery)
}

// clampToRuneStart backs an end offset up to the start of the rune containing
// it, so escaped[:n] never ends mid-rune.
func clampToRuneStart(s string, n int) int {
	for n > 0 && n < len(s) && !utf8.RuneStart(s[n]) {
		n--
	}
	return n
}

// highlightQuery wraps all occurrences of query in the snippet with <mark> tags.
// Both snippet and query must already be HTML-escaped.
func highlightQuery(snippet, query string) string {
	if query == "" {
		return snippet
	}
	return strings.ReplaceAll(snippet, query, "<mark>"+query+"</mark>")
}
