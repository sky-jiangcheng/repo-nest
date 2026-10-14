package db

import (
	"database/sql"
	"fmt"
)

// Graph-aware evidence support (ADR-0018 lane 2, schema v23). The read side of the
// knowledge graph: expand a seed set of pages along a whitelist of relations, so
// evidence gathering can surface a hit's neighbours that never matched the query
// themselves.
//
// The whitelist is deliberately narrow — ref / part-of / documents — and pointedly
// EXCLUDES contradicts. A contradiction is shown to a human as "these two pages
// disagree"; folding it into the same evidence block as the thing it contradicts
// would feed the model both sides of a dispute with no signal that they conflict,
// which is worse than omitting it. depends / implements / supersedes / mentions are
// also excluded from expansion for now: they are queryable (the v21 vocabulary
// exists for that) but expanding recall along them is a hypothesis the abeval gate
// has not tested, so the default walk stays on the three "structural containment"
// relations. Widening the whitelist is a one-line change gated on evidence.
//
// It walks only APPROVED pages at both ends: an edge into or out of a pending page
// must never pull unreviewed content into evidence (ADR-0015), and the seed set is
// already approved-only (it comes from FTS/vector recall, both approved-filtered).
//
// The recursion is bounded in two independent ways — a hop count (maxHops) and a
// visited guard that stops re-entering a node already on the current path — so a
// cycle in page_links cannot make the CTE run away. SQLite's recursive CTE has no
// native depth limit, so the path string + hop counter ARE the bound.

// graphExpandRelations is the whitelist the recursive walk follows. Kept as a
// package var (not inlined into the SQL string) so a test can assert it and a
// future widening is a single, reviewable edit. The SQL IN-list is built from it.
var graphExpandRelations = []string{
	RelationRef,
	RelationPartOf,
	RelationDocuments,
}

// GraphNeighbor is one page reached by expansion, with the hop distance from the
// nearest seed and the relation that led there.
type GraphNeighbor struct {
	PageID   int64
	Hops     int
	Relation string
}

// WikiGraphNeighbors expands from `seeds` along the relation whitelist up to
// `maxHops` hops, returning each reachable approved page (excluding the seeds
// themselves) with its shortest hop distance. It is bounded and cycle-safe.
//
// Returns an empty slice (no error) when seeds is empty — a query with no recall
// has nothing to expand, which is a normal outcome, not a failure.
func WikiGraphNeighbors(db *sql.DB, seeds []int64, maxHops int) ([]GraphNeighbor, error) {
	if len(seeds) == 0 || maxHops < 1 {
		return nil, nil
	}
	if maxHops > 4 {
		maxHops = 4 // hard ceiling: deeper walks are never justified by evidence quality
	}

	// Build the IN-list for seeds and the relation whitelist from Go vars, so the
	// two live in one place each. Values are validated ints / canonical relations,
	// so string interpolation here is not an injection surface.
	seedPH := inPlaceholders(len(seeds))
	seedArgs := make([]any, len(seeds))
	for i, s := range seeds {
		seedArgs[i] = s
	}
	relPH := inPlaceholders(len(graphExpandRelations))
	relArgs := make([]any, len(graphExpandRelations))
	for i, r := range graphExpandRelations {
		relArgs[i] = r
	}

	// The recursive CTE carries a `path` (comma-joined visited ids) and `hops`.
	// The path guard (the neighbour is not already in the path) is what makes a
	// cycle terminate; the hop guard caps depth. The JOIN to wiki_pages on BOTH
	// ends with status='approved' keeps pending pages out of the walk entirely — an
	// edge touching a pending page is simply not followed.
	query := fmt.Sprintf(`
WITH RECURSIVE walk(page_id, hops, relation, path) AS (
	SELECT l.to_page_id, 1, l.relation, ',' || l.from_page_id || ',' || l.to_page_id || ','
	  FROM page_links l
	  JOIN wiki_pages fp ON fp.id = l.from_page_id AND fp.status = 'approved'
	  JOIN wiki_pages tp ON tp.id = l.to_page_id AND tp.status = 'approved'
	 WHERE l.from_page_id IN (%s) AND l.relation IN (%s)
	UNION ALL
	SELECT l.to_page_id, w.hops + 1, l.relation, w.path || l.to_page_id || ','
	  FROM walk w
	  JOIN page_links l ON l.from_page_id = w.page_id AND l.relation IN (%s)
	  JOIN wiki_pages tp ON tp.id = l.to_page_id AND tp.status = 'approved'
	 WHERE w.hops < ?
	   AND instr(w.path, ',' || l.to_page_id || ',') = 0
)
SELECT page_id, MIN(hops) AS hops, relation
  FROM walk
 WHERE page_id NOT IN (%s)
 GROUP BY page_id, relation
 ORDER BY hops, relation, page_id`, seedPH, relPH, relPH, seedPH)

	args := make([]any, 0, len(seedArgs)+len(relArgs)+1+len(seedArgs))
	args = append(args, seedArgs...)
	args = append(args, relArgs...)
	args = append(args, relArgs...)
	args = append(args, maxHops)
	args = append(args, seedArgs...)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: wiki graph neighbors: %w", err)
	}
	defer rows.Close()

	var out []GraphNeighbor
	for rows.Next() {
		var n GraphNeighbor
		if err := rows.Scan(&n.PageID, &n.Hops, &n.Relation); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// WikiAdjacency loads the directed approved-page subgraph over the whitelist,
// suitable for feeding graph.PPR. Only approved pages at both ends are included.
// `seeds` restricts the loaded neighbourhood to nodes reachable from the seeds (so
// PPR runs on a small local subgraph, not the whole library), and `limit` bounds
// the number of edges loaded to keep a pathological hub from pulling an unbounded
// graph into memory.
func WikiAdjacency(db *sql.DB, seeds []int64, maxHops, limit int) (map[int64][]int64, error) {
	if len(seeds) == 0 || maxHops < 1 || limit <= 0 {
		return nil, nil
	}
	nbrs, err := WikiGraphNeighbors(db, seeds, maxHops)
	if err != nil {
		return nil, err
	}
	// The node set is the seeds plus everything reached; we then load the out-edges
	// among exactly those nodes, so PPR sees a closed subgraph.
	nodeSet := map[int64]bool{}
	for _, s := range seeds {
		nodeSet[s] = true
	}
	for _, n := range nbrs {
		nodeSet[n.PageID] = true
	}
	if len(nodeSet) == 0 {
		return nil, nil
	}

	nodePH := inPlaceholders(len(nodeSet))
	relPH := inPlaceholders(len(graphExpandRelations))
	// The query references the node IN-list twice (from AND to), so the node args
	// are bound twice, in that order, before the relation args and the limit.
	nodeArgs := make([]any, 0, len(nodeSet))
	for id := range nodeSet {
		nodeArgs = append(nodeArgs, id)
	}
	relArgs := make([]any, len(graphExpandRelations))
	for i, r := range graphExpandRelations {
		relArgs[i] = r
	}
	args := make([]any, 0, 2*len(nodeArgs)+len(relArgs)+1)
	args = append(args, nodeArgs...)
	args = append(args, nodeArgs...)
	args = append(args, relArgs...)
	args = append(args, limit)

	rows, err := db.Query(fmt.Sprintf(`
SELECT l.from_page_id, l.to_page_id
  FROM page_links l
  JOIN wiki_pages fp ON fp.id = l.from_page_id AND fp.status = 'approved'
  JOIN wiki_pages tp ON tp.id = l.to_page_id AND tp.status = 'approved'
 WHERE l.from_page_id IN (%s) AND l.to_page_id IN (%s) AND l.relation IN (%s)
 LIMIT ?`, nodePH, nodePH, relPH), args...)
	if err != nil {
		return nil, fmt.Errorf("db: wiki adjacency: %w", err)
	}
	defer rows.Close()

	adj := map[int64][]int64{}
	for rows.Next() {
		var from, to int64
		if err := rows.Scan(&from, &to); err != nil {
			return nil, err
		}
		adj[from] = append(adj[from], to)
	}
	return adj, rows.Err()
}

// inPlaceholders renders "?,?,?" for n values. n<=0 yields an empty string; callers
// guard against that before building a query.
func inPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, 0, 2*n)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
	}
	return string(b)
}
