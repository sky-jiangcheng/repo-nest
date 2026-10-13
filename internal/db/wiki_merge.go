package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Page merge (ADR-0019 决策 5, schema unchanged).
//
// ADR-0015 told a human "concepts will split, you merge them" and shipped no way
// to do it: the review surface had approve / reject / delete and the whole repo
// had no merge primitive. This is that primitive.
//
// The invariant the entire design hangs on is that a merge is an ABSORPTION, not
// an edit. The winner's row is never touched — not its content, not its slug,
// not its updated_at — so "编译器与任何自动路径不得修改已批准页" (ADR-0015 决策 3)
// survives the merge tool existing. What changes is the loser and the graph
// around it, which is why the operation is expressible as: redirect edges, move
// provenance, file a tombstone.
//
// No migration. ADR-0019 决策 6 contemplates an optional `page_merges` audit
// table for undo, and says explicitly not to build it until a real withdrawal
// request exists. Nothing here needs it: the loser's row IS the audit record
// (its tombstone names the winner), and page_versions (v22) already froze
// whatever the loser looked like while it was approved.
//
// Note the loser keeps its slug forever, exactly like a rejected page does.
// Freeing it would mean renaming or deleting the row, and the reason
// reject≠delete exists — a vanished page takes its inbound edges with it and
// turns every page that cited it into a dead link — applies verbatim here.

// MergeResult reports what an absorption actually moved. The counts are the
// review surface's evidence that the operation did something, and "0 edges,
// 0 notes" is how a replay of an already-finished merge shows itself.
type MergeResult struct {
	LoserID             int64  `json:"loser_id"`
	LoserSlug           string `json:"loser_slug"`
	WinnerID            int64  `json:"winner_id"`
	WinnerSlug          string `json:"winner_slug"`
	EdgesRedirected     int    `json:"edges_redirected"`
	EdgesDropped        int    `json:"edges_dropped"`
	NotesReattached     int    `json:"notes_reattached"`
	LoserPreviousStatus string `json:"loser_previous_status"`
}

// Merge refusal reasons. They are distinct error values because the review page
// has to say WHY a merge is not available rather than disabling a button with no
// explanation — the same reason ErrWikiSlugTaken exists.
var (
	ErrMergeSelfLink     = errors.New("cannot merge a page into itself")
	ErrMergeCrossProject = errors.New("pages belong to different projects")
	ErrMergeWinnerDead   = errors.New("the winner must be an approved or pending page")
	ErrMergeNothing      = errors.New("merge requires a tombstone body for the absorbed page")
)

// MergeWikiPages absorbs loserID into winnerID in one transaction.
//
// tombstone is the body the loser is replaced with ("已并入 [[winner]]"); it is a
// parameter rather than a constant here because the sentence is user-facing copy
// and belongs to the service layer's language, while this function owns the
// ordering. An empty tombstone is refused: silently blanking a page is exactly
// the information loss this whole file is shaped to avoid.
//
// Order of operations, and why it is not interchangeable:
//
//  1. the loser's edges and note attachments are READ first, into memory;
//  2. they are then cleared from the loser and re-inserted on the winner, which is
//     what makes "loser 出入边为空" (晋升条件 2b) true without a window in which a
//     page is attached to two rows at once;
//  3. the tombstone is written LAST, because it is the visible "this happened"
//     marker and it must never be able to exist on a page whose data was only
//     half-moved. All of it is one transaction, so a crash leaves the pre-merge
//     page and graph intact.
//
// Every copy is INSERT OR IGNORE against page_links'
// UNIQUE(from_page_id, to_page_id,relation) and note_pages' PK(note_id, page_id),
// which is what makes a replay idempotent: the second run of an already-completed
// merge moves nothing and reports zeros.
func MergeWikiPages(db *sql.DB, loserID, winnerID int64, tombstone string) (*MergeResult, error) {
	if loserID == winnerID {
		return nil, ErrMergeSelfLink
	}
	if strings.TrimSpace(tombstone) == "" {
		return nil, ErrMergeNothing
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	loser, err := txGetPage(tx, loserID)
	if err != nil {
		return nil, fmt.Errorf("db: merge loser: %w", err)
	}
	winner, err := txGetPage(tx, winnerID)
	if err != nil {
		return nil, fmt.Errorf("db: merge winner: %w", err)
	}
	// 待决 4: no cross-project merges in the MVP. Absorbing a page from project B
	// into project A would silently move B's knowledge out of B's view, and B's
	// user never agreed to it. A global page (project_id 0) may take either role.
	if loser.ProjectID != winner.ProjectID && loser.ProjectID > 0 && winner.ProjectID > 0 {
		return nil, fmt.Errorf("db: %w (loser in project %d, winner in project %d)",
			ErrMergeCrossProject, loser.ProjectID, winner.ProjectID)
	}
	// A rejected page cannot receive anything: its body is a tombstone by
	// definition, so redirecting the graph into it would hide every citation.
	if winner.Status == WikiStatusRejected {
		return nil, ErrMergeWinnerDead
	}

	res := &MergeResult{
		LoserID: loser.ID, LoserSlug: loser.Slug,
		WinnerID: winner.ID, WinnerSlug: winner.Slug,
		LoserPreviousStatus: loser.Status,
	}

	// 1. Outgoing edges: loser -> x becomes winner -> x.
	//
	// The loser->winner edge (in either direction) is DISCARDED rather than
	// redirected: rewritten it would be winner -> winner, which page_links'
	// CHECK (from_page_id <> to_page_id) rightly refuses. Dropping it is also
	// semantically correct — a page does not cite the page it became.
	outRows, err := tx.Query(
		`SELECT to_page_id, relation FROM page_links WHERE from_page_id = ? ORDER BY relation, to_page_id`, loserID)
	if err != nil {
		return nil, fmt.Errorf("db: merge read outgoing edges: %w", err)
	}
	type edge struct {
		other    int64
		relation string
	}
	var out, in []edge
	for outRows.Next() {
		var e edge
		if err := outRows.Scan(&e.other, &e.relation); err != nil {
			outRows.Close()
			return nil, err
		}
		out = append(out, e)
	}
	if err := outRows.Err(); err != nil {
		outRows.Close()
		return nil, err
	}
	outRows.Close()

	// 2. Incoming edges: x -> loser becomes x -> winner. This is the direction the
	// merge exists for: every page that cited the loser must now cite the winner,
	// or the absorption silently destroys the graph's backlink property.
	inRows, err := tx.Query(
		`SELECT from_page_id, relation FROM page_links WHERE to_page_id = ? ORDER BY relation, from_page_id`, loserID)
	if err != nil {
		return nil, fmt.Errorf("db: merge read incoming edges: %w", err)
	}
	for inRows.Next() {
		var e edge
		if err := inRows.Scan(&e.other, &e.relation); err != nil {
			inRows.Close()
			return nil, err
		}
		in = append(in, e)
	}
	if err := inRows.Err(); err != nil {
		inRows.Close()
		return nil, err
	}
	inRows.Close()

	if _, err := tx.Exec(`DELETE FROM page_links WHERE from_page_id = ? OR to_page_id = ?`, loserID, loserID); err != nil {
		return nil, fmt.Errorf("db: merge clear loser edges: %w", err)
	}
	for _, e := range out {
		if err := mergeCopyEdge(tx, winnerID, e.other, e.relation, res); err != nil {
			return nil, err
		}
	}
	for _, e := range in {
		if err := mergeCopyEdge(tx, e.other, winnerID, e.relation, res); err != nil {
			return nil, err
		}
	}

	// 3. Provenance: the notes the loser was compiled from now feed the winner.
	// Read-then-delete rather than a single UPDATE so a note already attached to
	// the winner collapses under the PK instead of erroring the transaction —
	// which is the normal case, not the exceptional one: two split pages compiled
	// from overlapping sources is precisely how the split got noticed.
	noteRows, err := tx.Query(`SELECT note_id FROM note_pages WHERE page_id = ? ORDER BY note_id`, loserID)
	if err != nil {
		return nil, fmt.Errorf("db: merge read loser notes: %w", err)
	}
	var noteIDs []int64
	for noteRows.Next() {
		var id int64
		if err := noteRows.Scan(&id); err != nil {
			noteRows.Close()
			return nil, err
		}
		noteIDs = append(noteIDs, id)
	}
	if err := noteRows.Err(); err != nil {
		noteRows.Close()
		return nil, err
	}
	noteRows.Close()
	if _, err := tx.Exec(`DELETE FROM note_pages WHERE page_id = ?`, loserID); err != nil {
		return nil, fmt.Errorf("db: merge clear loser notes: %w", err)
	}
	for _, id := range noteIDs {
		if _, err := tx.Exec(`INSERT INTO note_pages(note_id, page_id) VALUES (?, ?)
			ON CONFLICT DO NOTHING`, id, winnerID); err != nil {
			return nil, fmt.Errorf("db: merge reattach note %d: %w", id, err)
		}
		// Counted even when the row already existed: provenance-wise the note is
		// now attached to the winner, which is the fact being reported.
		res.NotesReattached++
	}

	// 4. Tombstone last. SetWikiPageStatus is deliberately not used: it would be a
	// second UPDATE on the same row, and this must be one write so the status and
	// the body can never be observed disagreeing.
	if _, err := tx.Exec(
		`UPDATE wiki_pages SET status = ?, content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		WikiStatusRejected, tombstone, loserID); err != nil {
		return nil, fmt.Errorf("db: file merge tombstone: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

// mergeCopyEdge redirects one edge onto the winner, accounting for the two ways
// it does not survive: it points at the winner itself, or the winner already has
// that exact edge (UNIQUE absorbs it, which is ADR-0019 决策 5's "winner 已有同边
// 则被 UNIQUE 丢弃").
func mergeCopyEdge(tx *sql.Tx, fromID, toID int64, relation string, res *MergeResult) error {
	if fromID == toID {
		res.EdgesDropped++
		return nil
	}
	rn, err := tx.Exec(`INSERT INTO page_links(from_page_id, to_page_id, relation) VALUES (?,?,?)
		ON CONFLICT(from_page_id, to_page_id, relation) DO NOTHING`, fromID, toID, relation)
	if err != nil {
		return fmt.Errorf("db: merge redirect edge %d->%d (%s): %w", fromID, toID, relation, err)
	}
	n, err := rn.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		res.EdgesDropped++
	} else {
		res.EdgesRedirected++
	}
	return nil
}

func txGetPage(tx *sql.Tx, id int64) (*WikiPage, error) {
	var p WikiPage
	err := scanPageFields(tx.QueryRow("SELECT "+pageColumns+" FROM wiki_pages WHERE id = ?", id), &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// PagePairCount is one blocking hit: two pages that share a source note, with the
// number of notes in common. It exists so candidate discovery (ADR-0019 决策 2)
// can get its "same origin" signal from one indexed query instead of an O(n²)
// loop in Go — which is the difference between "runs on a real library" and
// "runs on a fixture".
//
// a.page_id < b.page_id is what keeps each unordered pair to one row; without it
// the self-join returns both orders. A page sharing a note with itself is excluded
// by the same predicate.
type PagePairCount struct {
	Left  int64 `json:"left"`
	Right int64 `json:"right"`
	Count int   `json:"count"`
}

// NoteOverlapPairs returns pages that were compiled from overlapping notes.
// projectID 0 means "every project's pages plus the global ones", and the pair is
// then filtered to same-project-or-global here, so the service never has to
// re-derive the cross-project rule and cannot forget it.
func NoteOverlapPairs(db *sql.DB, projectID int64) ([]PagePairCount, error) {
	q := `SELECT a.page_id, b.page_id, COUNT(*)
	        FROM note_pages a
	        JOIN note_pages b ON a.note_id = b.note_id AND a.page_id < b.page_id
	        JOIN wiki_pages wl ON wl.id = a.page_id
	        JOIN wiki_pages wr ON wr.id = b.page_id
	       WHERE (wl.project_id IS wr.project_id
	              OR COALESCE(wl.project_id, 0) = 0
	              OR COALESCE(wr.project_id, 0) = 0)`
	args := []any{}
	if projectID > 0 {
		q += " AND (COALESCE(wl.project_id, 0) = ? OR COALESCE(wr.project_id, 0) = ?)"
		args = append(args, projectID, projectID)
	}
	q += " GROUP BY a.page_id, b.page_id ORDER BY COUNT(*) DESC, a.page_id, b.page_id"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("db: note overlap pairs: %w", err)
	}
	defer rows.Close()
	var out []PagePairCount
	for rows.Next() {
		var p PagePairCount
		if err := rows.Scan(&p.Left, &p.Right, &p.Count); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
