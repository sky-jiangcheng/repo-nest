// Package plugin defines the knowledge-source SPI for Repotory.
//
// The in-process script-plugin system (ADR-0002, yaegi interpreter) has been
// removed (C11 方案B): there is no user-authored plugin loader anymore. What
// remains is the small interface every built-in knowledge importer implements
// (claude, codex, opencode, openclaw, hermes, cursor, codebuddy,
// codebuddy-memory) and the document shape it produces. Registration, gating
// and the idempotent upsert path live in internal/core/plugin/runtime;
// docs/plugins/overview.md is the user-facing list of sources.
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

// ProbeResult is the outcome of a knowledge source's presence check: whether
// the agent's data actually exists on this machine, plus a short
// human-readable detail line (root path, match count) for the settings page.
// A probe is cheap and read-only — stat/readdir on the source's own
// allowlisted directory, never file content, never parent/sibling paths
// (ADR-0011 决策 3 hard scoping, ADR-0022 决策 1).
type ProbeResult struct {
	Detected bool   `json:"detected"`
	Detail   string `json:"detail"`
}

// Prober is an OPTIONAL capability of a KnowledgeImporter. Sources that
// implement it let the runtime report "which agent memory exists on this
// machine" in the settings page instead of listing every registered source
// blindly. Probe must stay cheap and must never read data outside the
// source's own declared directory.
type Prober interface {
	Probe() ProbeResult
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
