// Package plugin defines the extension interfaces for RepoNest plugins.
//
// Plugins are loaded in-process at startup. Each plugin implements Plugin,
// receives a PluginContext during Init, and may register event handlers and
// knowledge importers. See ADR 0002 for the in-process plugin decision.
//
// Note: the original RFC 0001 / issue #32 sketch referenced kb.Facade and
// storage.Stores abstractions that do not exist in the current codebase
// (see issue #38). This interface is therefore grounded in the existing
// internal/db layer to avoid resurrecting unused abstraction.
package plugin

import (
	"database/sql"
)

// Plugin is the base interface every RepoNest plugin must implement.
type Plugin interface {
	// Name returns the stable identifier of the plugin.
	Name() string
	// Init is called once at startup with the runtime context. The plugin
	// should register event handlers and knowledge sources here.
	Init(ctx PluginContext) error
}

// PluginContext provides a plugin access to the application's core services
// during initialization and at runtime.
type PluginContext interface {
	// DB exposes the underlying SQLite handle for read/write access.
	DB() *sql.DB
	// On registers a handler for a named runtime event
	// (e.g. OnNoteCreated / OnProjectScanned / OnImportRequested).
	On(event string, handler EventHandler)
	// RegisterKnowledgeSource registers a knowledge importer so it can be
	// triggered via TriggerImport(name).
	RegisterKnowledgeSource(name string, importer KnowledgeImporter)
}

// Event is a runtime event delivered to registered handlers.
type Event struct {
	// Name is the event identifier, e.g. "note.created".
	Name string
	// Data carries optional event payload.
	Data any
}

// EventHandler processes a single event. Errors are logged by the runtime;
// a panicking handler must not crash the host process.
type EventHandler func(event Event) error

// KnowledgeImporter imports knowledge (e.g. notes) from an external source
// into the RepoNest knowledge base.
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
