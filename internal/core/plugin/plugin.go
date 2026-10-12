// Package plugin defines the knowledge-source SPI for Repotory.
//
// The in-process script-plugin system (ADR-0002, yaegi interpreter) has been
// removed (C11 方案B): there is no user-authored plugin loader anymore. What
// remains is the small interface every built-in knowledge importer implements
// (claude, codex, opencode, openclaw, hermes, cursor) and the document shape
// it produces. Registration, gating and the idempotent upsert path live in
// internal/core/plugin/runtime; docs/plugins/overview.md is the user-facing
// list of sources.
package plugin

// KnowledgeImporter imports knowledge (e.g. notes) from an external source
// into the Repotory knowledge base.
type KnowledgeImporter interface {
	// Source returns a stable source identifier, e.g. "claude".
	Source() string
	// Import fetches documents from the external source. Implementations
	// should be idempotent: re-running updates existing notes rather than
	// duplicating them.
	Import() ([]ImportDoc, error)
}

// ImportDoc is a single document produced by a KnowledgeImporter, to be
// upserted into the knowledge base.
type ImportDoc struct {
	ProjectID int64  // target project; 0 to skip
	Title     string // note title
	Content   string // note body (Markdown)
	Tags      string // comma-separated tags
	Kind      string // note kind: knowledge | log | idea | other
	Source    string // provenance, e.g. "claude"
}
