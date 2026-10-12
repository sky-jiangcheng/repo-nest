package db

import (
	"database/sql"
	"fmt"
)

// mergedPageBody replaces a loser's content after a merge (ADR-0019 决策 5).
// The row is kept — deleting it would cascade away the edges still pointing at
// it and break the audit trail — so the body becomes a signpost to the winner.
const mergedPageBody = "✅ 此页已并入 [[%s]]，其边与来源挂接已由该页承接。"

// MergeWikiPages folds loser into winner in one transaction (ADR-0019 决策 5).
// The merge is human-triggered by contract: no compiler/lint/model path calls
// it. What it does, in order:
//
//   - every inbound and outbound edge of loser is re-pointed at winner
//     (INSERT OR IGNORE so an edge winner already has is absorbed by the
//     UNIQUE triple instead of erroring; a loser→winner edge re-pointed at
//     winner→winner is dropped by the from<>to CHECK, also via OR IGNORE);
//   - loser's note attachments move to winner (provenance is not lost);
//   - loser becomes a rejected tombstone whose body points at the winner
//     (reject≠delete, the ADR-0015 discipline);
//   - winner's id/slug/content/updated_at are NOT touched — the merge absorbs
//     the loser, it does not edit the winner.
//
// Replaying the same pair is idempotent: the loser has no edges or
// attachments left, and rewriting the identical tombstone snapshots nothing.
func MergeWikiPages(db *sql.DB, winnerID, loserID int64) error {
	if winnerID == loserID {
		return fmt.Errorf("db: refusing to merge page %d into itself", winnerID)
	}
	winner, err := GetWikiPageByID(db, winnerID)
	if err != nil {
		return fmt.Errorf("db: merge winner %d: %w", winnerID, err)
	}
	loser, err := GetWikiPageByID(db, loserID)
	if err != nil {
		return fmt.Errorf("db: merge loser %d: %w", loserID, err)
	}
	if winner.Status == WikiStatusRejected {
		return fmt.Errorf("db: refusing to merge into rejected page %d", winnerID)
	}
	// ADR-0019 待决 4: no cross-project merges in the MVP. Re-pointing edges
	// across projects would make the loser's project silently lose knowledge.
	if winner.ProjectID != loser.ProjectID && winner.ProjectID != 0 && loser.ProjectID != 0 {
		return fmt.Errorf("db: refusing a cross-project merge (page %d project %d, page %d project %d)",
			winnerID, winner.ProjectID, loserID, loser.ProjectID)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Re-point loser's outbound edges. OR IGNORE absorbs both the UNIQUE
	// triple (winner already has this edge) and the from<>to CHECK (the edge
	// pointed at the winner itself, so re-pointing would be a self-loop).
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO page_links(from_page_id, to_page_id, relation)
		 SELECT ?, to_page_id, relation FROM page_links WHERE from_page_id = ?`,
		winnerID, loserID); err != nil {
		return fmt.Errorf("db: re-point outbound edges of page %d: %w", loserID, err)
	}
	// Re-point loser's inbound edges (same absorption rules, other direction).
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO page_links(from_page_id, to_page_id, relation)
		 SELECT from_page_id, ?, relation FROM page_links WHERE to_page_id = ?`,
		winnerID, loserID); err != nil {
		return fmt.Errorf("db: re-point inbound edges of page %d: %w", loserID, err)
	}
	// Drop the loser's original rows — their re-pointed copies now live under
	// the winner (the absorbed ones were never inserted).
	if _, err := tx.Exec(
		`DELETE FROM page_links WHERE from_page_id = ? OR to_page_id = ?`,
		loserID, loserID); err != nil {
		return fmt.Errorf("db: drop re-pointed edges of page %d: %w", loserID, err)
	}

	// Move provenance: loser's source notes attach to the winner; the PK
	// (note_id, page_id) absorbs the ones the winner already has.
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO note_pages(note_id, page_id)
		 SELECT note_id, ? FROM note_pages WHERE page_id = ?`,
		winnerID, loserID); err != nil {
		return fmt.Errorf("db: move note attachments of page %d: %w", loserID, err)
	}
	if _, err := tx.Exec(`DELETE FROM note_pages WHERE page_id = ?`, loserID); err != nil {
		return fmt.Errorf("db: drop loser note attachments: %w", err)
	}

	// Tombstone the loser: same body-replacement discipline as
	// RejectWikiPage, but the notice points at the surviving page. Written
	// through the normal update path, so the v23 snapshot trigger records the
	// tombstone as the loser's newest state.
	//
	// The trigger snapshots the NEW state (the v8/v11 convention), so the
	// displaced body would leave no trace without this explicit pre-image —
	// the same reason RestorePageVersion snapshots before applying. A merge
	// must not silently destroy the loser's content.
	if _, err := tx.Exec(
		`INSERT INTO page_versions(page_id, title, content, kind)
		 SELECT id, title, content, kind FROM wiki_pages WHERE id = ?`,
		loserID); err != nil {
		return fmt.Errorf("db: snapshot page %d before tombstone: %w", loserID, err)
	}
	if _, err := tx.Exec(
		`UPDATE wiki_pages SET content = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		fmt.Sprintf(mergedPageBody, winner.Slug), WikiStatusRejected, loserID); err != nil {
		return fmt.Errorf("db: tombstone page %d: %w", loserID, err)
	}

	return tx.Commit()
}
