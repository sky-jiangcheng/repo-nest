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

// SearchWikiPages ranks APPROVED pages by bm25 over title+content — the status
// filter is ADR-0015's whole point: a page a human has not reviewed must never
// answer a question, appear in an evidence block, or surface in the knowledge
// search. The inventory paths (ListWikiPages, the export bypass, lint) still see
// every status, because pending rows are exactly what review and lint must look at.
//
// Empty query returns
// nothing rather than everything: callers use it as a retrieval stage, and an
// "everything" result would silently flood the evidence budget.
func SearchWikiPages(db *sql.DB, query string, limit int) ([]WikiPage, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	// Reuse the notes path's escapers verbatim: two independent FTS quoting rules
	// in one package is how a quote in a user query eventually breaks one path and
	// not the other.
	if !ftsUsable(query) {
		return nil, nil
	}
	phrase := escapeFTS(query)

	rows, err := db.Query(
		"SELECT "+pageColumnsP+", bm25(wiki_pages_fts, 3.0, 1.0) AS rank\n"+
			`   FROM wiki_pages_fts
		   JOIN wiki_pages p ON p.id = wiki_pages_fts.rowid
		  WHERE wiki_pages_fts MATCH ? AND p.status = 'approved'
		  ORDER BY rank
		  LIMIT ?`, phrase, limit)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 0 {
		return out, nil
	}

	// Same relaxation the notes path uses (ADR-0003 review): a multi-term AND
	// that matches nothing falls back to an OR of the terms, staying inside FTS so
	// a damaged index still returns nothing rather than pretending.
	loose := escapeFTSOR(query)
	if strings.TrimSpace(loose) == "" || loose == phrase {
		return out, nil
	}
	return SearchWikiPagesMatch(db, loose, limit)
}

// SearchWikiPagesMatch runs one raw MATCH expression; exported for the A/B gate
// so it can measure the strict and relaxed paths separately.
func SearchWikiPagesMatch(db *sql.DB, match string, limit int) ([]WikiPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := db.Query(
		"SELECT "+pageColumnsP+", bm25(wiki_pages_fts, 3.0, 1.0) AS rank\n"+
			`   FROM wiki_pages_fts
		   JOIN wiki_pages p ON p.id = wiki_pages_fts.rowid
		  WHERE wiki_pages_fts MATCH ? AND p.status = 'approved'
		  ORDER BY rank
		  LIMIT ?`, match, limit)
	if err != nil {
		return nil, fmt.Errorf("db: wiki page match: %w", err)
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
