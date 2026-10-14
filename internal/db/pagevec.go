package db

// Page-vector index — the wiki-layer twin of note_embeddings (vecindex.go), the
// first half of ADR-0018 lane 2 (schema v23).
//
// Why a PHYSICALLY SEPARATE vec0 table rather than pages sharing note_embeddings:
// hybrid.FuseRRF merges ranked lists of bare int64 ids. wiki_pages.id and
// project_notes.id come from independent AUTOINCREMENT sequences, so note #7 and
// page #7 are different objects that would fuse as one — the exact collision
// trap the evidence interleaver's comment calls out (wiki_evidence.go). A page
// vector therefore lives in page_embeddings, keyed by the page's own rowid, and
// RRF is only ever run within one type's id space; the two are interleaved as
// separate typed columns downstream, never fused together.
//
// It is a DERIVED cache, exactly like note_embeddings (ADR-0012 决策 5): wiki_pages
// stays the single source of truth and this can always be dropped and rebuilt. It
// exists only when semantic search is on — the page-embedding path hangs off the
// existing semantic_search switch rather than introducing its own, because that
// switch is the user's consent to send text to the embedding endpoint (see
// RebuildPageEmbeddings in the service layer). A second switch whose "off" state
// silently left the index stale is the failure mode the notes path already
// avoided, and copying pages would reproduce it.
//
// Only APPROVED pages are embedded. That mirrors SearchWikiPages' status filter
// (ADR-0015): a page a human has not reviewed must never surface in a vector
// recall that feeds AskAI evidence. The service layer enforces the same boundary
// at read time by listing only approved pages (ListPageEmbeddingInputs) and by
// re-checking status on every increment (NextDirtyPageEmbedWork).
import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite/vec"
)

// pageVecTableName is the page vector index. Separate from vecTableName so the
// two derived caches never share rows and page ids can collide with note ids
// harmlessly. Kept unquoted so vec0's `<table>_info`/`<table>_chunks` shadow
// tables follow the same name.
const pageVecTableName = "page_embeddings"

// pageVecMetaTable records the dimension the page index is built for, mirroring
// vecMetaTable: a model/dim swap must reliably rebuild, and sqlite-vec's shadow
// table columns are not a stable contract to introspect.
const pageVecMetaTable = "page_embeddings_meta"

// PageVectorIndexReady reports whether the page_embeddings vec0 table exists.
func PageVectorIndexReady(db *sql.DB) bool {
	var n int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", pageVecTableName).Scan(&n)
	return err == nil && n > 0
}

// EnsurePageVectorIndex creates the page vec0 table sized to dim, dropping a
// table of a DIFFERENT dim first (a model swap changes dim, so the cache is then
// rebuilt from scratch). dim must be 1..maxEmbeddingDim. Mirrors EnsureVectorIndex
// so the two indexes cannot drift in how they interpret a dim change.
func EnsurePageVectorIndex(db *sql.DB, dim int) error {
	if dim < 1 || dim > maxEmbeddingDim {
		return fmt.Errorf("db: invalid page embedding dim %d (want 1..%d)", dim, maxEmbeddingDim)
	}
	if PageVectorIndexReady(db) {
		if cur, ok := storedPageVectorDim(db); ok && cur == dim {
			return nil // already the right shape
		}
		if err := DropPageVectorIndex(db); err != nil {
			return err
		}
	}
	// dim is a validated int, so Sprintf injection is not a concern here.
	ddl := fmt.Sprintf("CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(embedding float[%d])", pageVecTableName, dim)
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("db: create page vec index: %w", err)
	}
	if _, err := db.Exec(
		"CREATE TABLE IF NOT EXISTS " + pageVecMetaTable + " (dim INTEGER NOT NULL)"); err != nil {
		return fmt.Errorf("db: create page vec meta: %w", err)
	}
	if _, err := db.Exec("DELETE FROM " + pageVecMetaTable); err != nil {
		return err
	}
	_, err := db.Exec("INSERT INTO "+pageVecMetaTable+"(dim) VALUES(?)", dim)
	return err
}

// storedPageVectorDim reads the tracked page-index dimension from its meta table.
func storedPageVectorDim(db *sql.DB) (dim int, ok bool) {
	err := db.QueryRow("SELECT dim FROM " + pageVecMetaTable + " LIMIT 1").Scan(&dim)
	if err != nil {
		return 0, false
	}
	return dim, true
}

// StoredPageVectorDim is the exported view of the page index's dimension, used by
// the page drainer when `embedding_dim` is unset: an index built before that key
// was persisted still knows its own width, and guessing wrong would write vectors
// that KNN silently mis-ranks. Twin of StoredVectorDim.
func StoredPageVectorDim(db *sql.DB) (int, bool) { return storedPageVectorDim(db) }

// DropPageVectorIndex removes the derived page vector table + its meta row.
func DropPageVectorIndex(db *sql.DB) error {
	if _, err := db.Exec("DROP TABLE IF EXISTS " + pageVecTableName); err != nil {
		return fmt.Errorf("db: drop page vec index: %w", err)
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS " + pageVecMetaTable); err != nil {
		return fmt.Errorf("db: drop page vec meta: %w", err)
	}
	return nil
}

// ClearPageVectorIndex empties the page vector table without dropping it (used
// before a full re-embed).
func ClearPageVectorIndex(db *sql.DB) error {
	if _, err := db.Exec("DELETE FROM " + pageVecTableName); err != nil {
		return fmt.Errorf("db: clear page vec index: %w", err)
	}
	return nil
}

// PutPageEmbedding upserts a page's vector, keyed by page id (the vec0 rowid).
func PutPageEmbedding(db *sql.DB, pageID int64, vec []float32) error {
	blob, err := encodeVector(vec)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		fmt.Sprintf("INSERT OR REPLACE INTO %s(rowid, embedding) VALUES(?, ?)", pageVecTableName),
		pageID, blob)
	if err != nil {
		return fmt.Errorf("db: put embedding page %d: %w", pageID, err)
	}
	return nil
}

// DeletePageEmbedding removes a page's vector (page deleted, or it left the
// approved set). sqlite-vec has no row for a page that was never embedded, so the
// DELETE is inherently idempotent.
func DeletePageEmbedding(db *sql.DB, pageID int64) error {
	_, err := db.Exec("DELETE FROM "+pageVecTableName+" WHERE rowid = ?", pageID)
	if err != nil {
		return fmt.Errorf("db: delete page embedding %d: %w", pageID, err)
	}
	return nil
}

// KnnPageIDs returns the page ids of the k vectors nearest to `query`, ordered by
// ascending distance (most similar first). Twin of KnnNoteIDs, and deliberately
// reading only page_embeddings so its ids are page ids that never collide with
// note ids.
func KnnPageIDs(db *sql.DB, query []float32, k int) ([]int64, error) {
	if k <= 0 {
		return nil, nil
	}
	blob, err := encodeVector(query)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(
		fmt.Sprintf("SELECT rowid FROM %s WHERE embedding MATCH ? AND k = ? ORDER BY distance", pageVecTableName),
		blob, k)
	if err != nil {
		return nil, fmt.Errorf("db: page knn: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// PageEmbeddingInput is one approved page's id + embeddable text for a rebuild or
// incremental pass.
type PageEmbeddingInput struct {
	ID   int64
	Text string
}

// PageEmbeddingText builds the exact string handed to the embedding endpoint for a
// page: title + content. Defined once so the full rebuild and the incremental
// drainer produce the same vector for the same page, matching NoteEmbeddingText's
// single-source contract (a page whose vector meant two things depending on which
// path wrote it would be ranked by RRF against a mix of representations).
func PageEmbeddingText(title, content string) string {
	if len(content) > MaxNoteContentLen {
		content = content[:MaxNoteContentLen]
	}
	return strings.TrimSpace(title + "\n" + content)
}

// ListPageEmbeddingInputs returns id + text for every APPROVED page, in id order,
// for a full re-embed. The status filter is the same rule SearchWikiPages applies
// (ADR-0015): only reviewed pages may ever be recalled, so only they are embedded.
func ListPageEmbeddingInputs(db *sql.DB) ([]PageEmbeddingInput, error) {
	rows, err := db.Query(
		"SELECT id, title, content FROM wiki_pages WHERE status = 'approved' ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("db: list page embedding inputs: %w", err)
	}
	defer rows.Close()
	var out []PageEmbeddingInput
	for rows.Next() {
		var (
			id      int64
			title   string
			content string
		)
		if err := rows.Scan(&id, &title, &content); err != nil {
			return nil, err
		}
		out = append(out, PageEmbeddingInput{ID: id, Text: PageEmbeddingText(title, content)})
	}
	return out, rows.Err()
}

// PageHitsByIDs loads APPROVED pages for the given ids, to materialise
// vector-recall ids the FTS pass did not return. Order is not guaranteed. It is
// the page twin of NoteHitsByIDs, and it re-applies the approved filter so a stale
// vector for a since-pending or since-deleted page can never enter evidence: the
// drainer removes such vectors, but a race (vector recall landing in the same
// window a page is un-approved) must not leak it.
func PageHitsByIDs(db *sql.DB, ids []int64) ([]WikiPage, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := db.Query(
		"SELECT "+pageColumnsP+" FROM wiki_pages p WHERE p.id IN ("+placeholders+") AND p.status = 'approved'",
		args...)
	if err != nil {
		return nil, fmt.Errorf("db: page hits by ids: %w", err)
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

// PrunePageEmbeddings removes every page vector whose page no longer exists OR is
// no longer approved, so a page that leaves the approved set (or is deleted
// through the project cascade) stops being recalled. Pages deleted through the
// project cascade have no single id to delete, so callers with a derived index
// present run this after the deleting transaction commits. Returns how many
// orphans were removed.
func PrunePageEmbeddings(db *sql.DB) (int64, error) {
	res, err := db.Exec(
		"DELETE FROM " + pageVecTableName +
			" WHERE rowid NOT IN (SELECT id FROM wiki_pages WHERE status = 'approved')")
	if err != nil {
		return 0, fmt.Errorf("db: prune page embeddings: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// PageVectorStoreHealthCheck verifies the page vec0 store works end to end:
// ensure the index at dim, write a probe vector at a sentinel rowid, confirm KNN
// returns it. Twin of VectorStoreHealthCheck so the page index gets the same
// end-to-end guarantee before it is ever trusted on the query path.
func PageVectorStoreHealthCheck(db *sql.DB, dim int) error {
	if dim < 1 || dim > maxEmbeddingDim {
		return fmt.Errorf("db: health check: invalid page dim %d", dim)
	}
	if err := EnsurePageVectorIndex(db, dim); err != nil {
		return err
	}
	probeRowid := int64(-1)
	probe := make([]float32, dim)
	for i := range probe {
		probe[i] = 0.5
	}
	if err := PutPageEmbedding(db, probeRowid, probe); err != nil {
		return err
	}
	ids, err := KnnPageIDs(db, probe, 1)
	if err != nil {
		return err
	}
	if len(ids) != 1 || ids[0] != probeRowid {
		return fmt.Errorf("db: health check page KNN returned %v, want probe %d", ids, probeRowid)
	}
	// Remove the probe so a health check never leaves a phantom page vector.
	_ = DeletePageEmbedding(db, probeRowid)
	return nil
}
