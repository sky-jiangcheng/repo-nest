package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// Page retrieval for the wiki layer (ADR-0014 决策 3 / TODO M6-W2).
//
// W1 shipped structure with no search, deliberately: search is the first thing a
// consumer needs, and W2 is that consumer. So page FTS arrives with W2 rather
// than being dead index in W1.
//
// Same shape as the notes/todos indexes: external-content FTS5 with the trigram
// tokenizer (case-insensitive substring matching for ASCII and CJK alike,
// ADR-0003) plus three sync triggers, so a page written through ANY path is
// immediately findable without a rebuild step. That "any path" property is the
// whole reason it is a trigger and not service code — W3's compiler will write
// pages through the same layer as everything else.
var wikiFTSSchemaStatements = []string{
	`CREATE VIRTUAL TABLE IF NOT EXISTS wiki_pages_fts USING fts5(` +
		`title, content, content='wiki_pages', content_rowid='id', tokenize='trigram')`,
	`CREATE TRIGGER IF NOT EXISTS wiki_pages_fts_ai AFTER INSERT ON wiki_pages BEGIN ` +
		`INSERT INTO wiki_pages_fts(rowid, title, content) VALUES (new.id, new.title, new.content); END`,
	`CREATE TRIGGER IF NOT EXISTS wiki_pages_fts_ad AFTER DELETE ON wiki_pages BEGIN ` +
		`INSERT INTO wiki_pages_fts(wiki_pages_fts, rowid, title, content) VALUES ('delete', old.id, old.title, old.content); END`,
	`CREATE TRIGGER IF NOT EXISTS wiki_pages_fts_au AFTER UPDATE ON wiki_pages BEGIN ` +
		`INSERT INTO wiki_pages_fts(wiki_pages_fts, rowid, title, content) VALUES ('delete', old.id, old.title, old.content); ` +
		`INSERT INTO wiki_pages_fts(rowid, title, content) VALUES (new.id, new.title, new.content); END`,
}

// wikiFTSObjects are the derived-index additions, listed separately from
// wikiObjects so DropWikiSchema's reversibility proof covers them too.
var wikiFTSObjects = []struct{ kind, name string }{
	{"TRIGGER", "wiki_pages_fts_ai"},
	{"TRIGGER", "wiki_pages_fts_au"},
	{"TRIGGER", "wiki_pages_fts_ad"},
	{"TABLE", "wiki_pages_fts"},
}

// EnsureWikiFTS creates the page index and its triggers. Idempotent, safe to
// call on any database that has wiki_pages (same contract as EnsureFTSIndex).
//
// The update trigger is AFTER UPDATE, not AFTER UPDATE OF ...: unlike the
// embedding queue (where a metadata-only write must not cost an HTTP request),
// an FTS row costs nothing, and an OF-restricted trigger would drift if a page
// column is ever added. Value comparison happens in the embedding queue precisely
// because that one has a price.
func EnsureWikiFTS(db *sql.DB) error {
	for _, stmt := range wikiFTSSchemaStatements {
		if _, err := db.Exec(stmt); err != nil {
			if isAlreadyExistsErr(err) {
				continue
			}
			return fmt.Errorf("db: ensure wiki FTS: %w", err)
		}
	}
	return nil
}

func dropWikiFTS(db *sql.DB) error {
	for _, o := range wikiFTSObjects {
		if _, err := db.Exec("DROP " + o.kind + " IF EXISTS " + o.name); err != nil {
			return fmt.Errorf("db: drop wiki FTS %s %s: %w", o.kind, o.name, err)
		}
	}
	return nil
}

// RebuildWikiFTS re-populates the page index from wiki_pages. Needed when the
// index was created after pages already existed (the triggers only cover writes
// from here on) and after the v7-style empty-index failure this repo has already
// lived through once (see migration v12).
func RebuildWikiFTS(db *sql.DB) error {
	if _, err := db.Exec(`INSERT INTO wiki_pages_fts(wiki_pages_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("db: rebuild wiki FTS: %w", err)
	}
	return nil
}

// SearchWikiPages is the retrieval ladder for pages, rung for rung the same
// shape as the notes path (ADR-0003 / M3-C), because a question about the
// user's knowledge must not come back empty for a reason that has nothing to
// do with the knowledge:
//
//  1. FTS strict AND over every term, gated on ftsUsable — the trigram
//     tokenizer extracts 3-character sequences, so a shorter term (every
//     2-character CJK word) cannot match anything at all;
//  2. FTS OR relaxation, ONLY when rung 1 returned zero rows, so relaxation
//     can fill an empty result set but never thin a good one (M3-C);
//  3. LIKE, for the short terms rung 1 cannot serve — every term first (a page
//     must contain them all), any term only when that AND pass is empty, the
//     same zero-row-only contract as rung 2.
//
// Every rung keeps the approved-only gate (ADR-0015): relaxation widens WHAT
// may match, never WHO may answer. An empty query returns nothing rather than
// everything: callers use this as a retrieval stage, and an "everything" result
// would silently flood the evidence budget.
func SearchWikiPages(db *sql.DB, query string, limit int) ([]WikiPage, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	if ftsUsable(query) {
		hits, err := searchWikiPagesFTS(db, escapeFTS(query), limit)
		if err == nil {
			if len(hits) > 0 {
				return hits, nil // rung 1: strict AND matched
			}
			// Rung 2 (M3-C): a zero-row strict AND relaxes to an OR of the
			// terms, staying inside FTS so a damaged index still yields
			// nothing rather than pretending.
			if orExpr := escapeFTSOR(query); orExpr != "" {
				if relaxed, rerr := searchWikiPagesFTS(db, orExpr, limit); rerr == nil {
					return relaxed, nil
				}
			}
			return hits, nil // empty AND result: preserve original behaviour
		}
		// Index missing or MATCH rejected: LIKE still serves the query, the
		// same fallback the notes path takes.
	}
	// Rung 3: LIKE covers the terms the trigram index cannot serve.
	if andHits, err := searchWikiPagesLike(db, query, limit, true); err == nil && len(andHits) > 0 {
		return andHits, nil
	}
	return searchWikiPagesLike(db, query, limit, false)
}

// searchWikiPagesFTS runs one prepared MATCH expression (escapeFTS for the
// strict rung, escapeFTSOR for the relaxed one) against the page index. Split
// out of SearchWikiPages so the A/B gate can drive each rung on its own via
// SearchWikiPagesMatch, and so the MATCH statement exists exactly once.
func searchWikiPagesFTS(db *sql.DB, matchExpr string, limit int) ([]WikiPage, error) {
	rows, err := db.Query(
		"SELECT "+pageColumnsP+", bm25(wiki_pages_fts, 3.0, 1.0) AS rank\n"+
			`   FROM wiki_pages_fts
		   JOIN wiki_pages p ON p.id = wiki_pages_fts.rowid
		  WHERE wiki_pages_fts MATCH ? AND p.status = 'approved'
		  ORDER BY rank, p.updated_at DESC, p.id ASC
		  LIMIT ?`, matchExpr, limit)
	if err != nil {
		// A rejected MATCH (unbalanced quotes and friends) must not be silent.
		return nil, fmt.Errorf("db: wiki page search: %w", err)
	}
	defer rows.Close()
	var out []WikiPage
	for rows.Next() {
		var (
			p    WikiPage
			rank float64
		)
		if err := scanPageFieldsWithRank(rows, &p, &rank); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// searchWikiPagesLike scans wiki_pages directly for the terms the trigram
// index cannot serve (anything under 3 runes — every 2-character CJK word).
// A title match outranks a content-only match, then recency, then id, so the
// seed order handed to the walk is fully deterministic: the graph pass breaks
// score ties by seed order, and an unstable seed order would make the same
// query rank differently run to run.
//
// requireAll ANDs the per-term conditions; the OR pass is only reached when
// the AND pass came back empty, so a page satisfying every term always wins
// over a page satisfying one.
func searchWikiPagesLike(db *sql.DB, query string, limit int, requireAll bool) ([]WikiPage, error) {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return nil, nil
	}
	joiner := " OR "
	if requireAll {
		joiner = " AND "
	}
	var (
		termConds  = make([]string, 0, len(terms))
		titleConds = make([]string, 0, len(terms))
		termArgs   = make([]any, 0, len(terms)*2)
		titleArgs  = make([]any, 0, len(terms))
	)
	for _, t := range terms {
		pat := "%" + escapeLike(t) + "%"
		termConds = append(termConds, "(p.title LIKE ? ESCAPE '\\' OR p.content LIKE ? ESCAPE '\\')")
		termArgs = append(termArgs, pat, pat)
		titleConds = append(titleConds, "p.title LIKE ? ESCAPE '\\'")
		titleArgs = append(titleArgs, pat)
	}
	// Args follow placeholder order in the statement text: the WHERE
	// per-term pairs first, then ORDER BY's title-priority CASE, then LIMIT.
	args := append(append(termArgs, titleArgs...), limit)
	rows, err := db.Query(
		"SELECT "+pageColumnsP+"\n"+
			"  FROM wiki_pages p\n"+
			" WHERE p.status = 'approved' AND ("+strings.Join(termConds, joiner)+")\n"+
			" ORDER BY CASE WHEN ("+strings.Join(titleConds, " OR ")+") THEN 0 ELSE 1 END,\n"+
			"          p.updated_at DESC, p.id ASC\n"+
			" LIMIT ?",
		args...)
	if err != nil {
		return nil, fmt.Errorf("db: wiki page like search: %w", err)
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

// SearchWikiPagesMatch runs one raw MATCH expression; exported for the A/B gate
// so it can measure the strict and relaxed paths separately.
func SearchWikiPagesMatch(db *sql.DB, match string, limit int) ([]WikiPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	return searchWikiPagesFTS(db, match, limit)
}
