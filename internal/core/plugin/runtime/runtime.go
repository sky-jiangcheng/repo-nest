// Package runtime implements the knowledge-source runtime for Repotory.
//
// There are no user-authored plugins: the yaegi script-plugin runtime was
// removed (C11 方案B, amending ADR-0002). What lives here is the registry the
// built-in knowledge importers register into (RegisterSource /
// RegisterSourceManual), the trigger/upsert path (TriggerImport -> upsertDoc)
// and the startup auto-import pass (ImportAll). The six built-in importers
// (claude, codex, opencode, openclaw, hermes, cursor) are the only tenants —
// see service.registerBuiltinImporters and docs/plugins/overview.md.
package runtime

import (
	"database/sql"
	"fmt"
	"log"
	"sync"

	"repotory/internal/core/plugin"
	"repotory/internal/db"
)

// SourceStatus describes a registered knowledge source for the settings page.
type SourceStatus struct {
	Name    string `json:"name"`
	Plugin  string `json:"plugin"`
	Enabled bool   `json:"enabled"`
}

// sourceEntry is a registered knowledge importer with its latest run result.
type sourceEntry struct {
	name     string
	plugin   string
	importFn func() ([]plugin.ImportDoc, error)
	imported int
	lastErr  error
	// auto marks sources eligible for the startup "import everything" pass
	// (ImportAll). Curated sources (claude memory) are auto; privacy-sensitive
	// transcript sources (codex, opencode, cursor, ...) are opt-in and only run
	// when triggered explicitly by name. See ADR-0011 决策 4.
	auto bool
}

// Runtime supervises the registered knowledge sources. It is safe for
// concurrent use.
type Runtime struct {
	mu      sync.Mutex
	db      *sql.DB
	sources map[string]*sourceEntry
	// importMu serializes TriggerImport so concurrent invocations of the same
	// source (e.g. UI button + startup auto-import) cannot double-import.
	importMu sync.Mutex
}

// New creates a Runtime bound to the given database handle.
func New(database *sql.DB) *Runtime {
	return &Runtime{
		db:      database,
		sources: make(map[string]*sourceEntry),
	}
}

// SourceStatuses returns the registered knowledge sources with their state.
func (r *Runtime) SourceStatuses() []SourceStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SourceStatus, 0, len(r.sources))
	for _, s := range r.sources {
		out = append(out, SourceStatus{
			Name:    s.name,
			Plugin:  s.plugin,
			Enabled: s.lastErr == nil,
		})
	}
	return out
}

// ImportRun summarizes a single knowledge import execution.
type ImportRun struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

// Total returns the number of documents processed.
func (r ImportRun) Total() int { return r.Created + r.Updated + r.Skipped }

// SourceRun pairs a knowledge source with its latest import run, returned by
// ImportAll for reporting on startup auto-imports.
type SourceRun struct {
	Name string    `json:"name"`
	Run  ImportRun `json:"run"`
	Err  string    `json:"error,omitempty"`
}

// ImportAll triggers every registered knowledge source and returns per-source
// results. Sources that fail are reported with Err set rather than aborting the
// remaining sources.
func (r *Runtime) ImportAll() []SourceRun {
	r.mu.Lock()
	names := make([]string, 0, len(r.sources))
	for name, src := range r.sources {
		// Skip opt-in sources (e.g. codex transcripts): the startup auto-import
		// pass runs curated sources only. Opt-in sources still run when
		// triggered explicitly by name via TriggerImport. ADR-0011 决策 4.
		if !src.auto {
			continue
		}
		names = append(names, name)
	}
	r.mu.Unlock()

	results := make([]SourceRun, 0, len(names))
	for _, name := range names {
		run, err := r.TriggerImport(name)
		sr := SourceRun{Name: name, Run: run}
		if err != nil {
			sr.Err = err.Error()
		}
		results = append(results, sr)
	}
	return results
}

// TriggerImport runs the knowledge source registered under name and upserts
// the returned documents into the knowledge base. Concurrent invocations are
// serialized via importMu to avoid duplicate imports.
func (r *Runtime) TriggerImport(name string) (ImportRun, error) {
	r.importMu.Lock()
	defer r.importMu.Unlock()

	r.mu.Lock()
	src := r.sources[name]
	r.mu.Unlock()
	if src == nil {
		return ImportRun{}, fmt.Errorf("plugin runtime: unknown knowledge source %q", name)
	}

	docs, err := r.safeImport(src)
	if err != nil {
		r.recordSourceError(src, err)
		return ImportRun{}, err
	}

	var run ImportRun
	var firstErr error
	for _, d := range docs {
		if d.ProjectID <= 0 {
			run.Skipped++
			continue
		}
		created, err := r.upsertDoc(d, src.name)
		if err != nil {
			log.Printf("plugin runtime: import %q doc %q failed: %v", name, d.Title, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if created {
			run.Created++
		} else {
			run.Updated++
		}
	}

	r.mu.Lock()
	src.imported = run.Total()
	src.lastErr = firstErr
	r.mu.Unlock()
	log.Printf("plugin runtime: import %q -> created %d, updated %d, skipped %d", name, run.Created, run.Updated, run.Skipped)
	return run, nil
}

// recordSourceError stores a source-level failure so SourceStatus.Enabled
// reflects it instead of always reporting healthy.
func (r *Runtime) recordSourceError(src *sourceEntry, err error) {
	r.mu.Lock()
	src.lastErr = err
	r.mu.Unlock()
}

// upsertDoc creates or updates a note keyed by (project, source, title).
// Returns true when the note was created, false when updated.
func (r *Runtime) upsertDoc(doc plugin.ImportDoc, source string) (bool, error) {
	kind := doc.Kind
	if kind == "" {
		kind = "knowledge"
	}
	existing, err := db.GetNoteBySourceTitle(r.db, doc.ProjectID, source, doc.Title)
	if err == nil {
		if doc.Content != "" {
			// UpdateNoteFull runs content + metadata in a single transaction, so
			// the note_versions snapshot fires once with consistent data and a
			// metadata failure can never leave content updated but meta stale.
			if uerr := db.UpdateNoteFull(r.db, existing.ID, doc.Content, doc.Title, doc.Tags, kind, existing.Pinned); uerr != nil {
				log.Printf("plugin runtime: upsertDoc UpdateNoteFull failed (note %d): %v", existing.ID, uerr)
				return false, uerr
			}
		} else if merr := db.UpdateNoteMeta(r.db, existing.ID, doc.Title, doc.Tags, kind, existing.Pinned); merr != nil {
			log.Printf("plugin runtime: upsertDoc UpdateNoteMeta failed (note %d): %v", existing.ID, merr)
			return false, merr
		}
		return false, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	_, err = db.CreateNoteEx(r.db, doc.ProjectID, doc.Title, doc.Content, doc.Tags, kind, source)
	if err != nil {
		return false, err
	}
	return true, nil
}

// safeImport runs an importer, recovering any panic.
func (r *Runtime) safeImport(src *sourceEntry) (docs []plugin.ImportDoc, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			docs = nil
			err = fmt.Errorf("plugin runtime: import %q panicked: %v", src.name, rec)
		}
	}()
	return src.importFn()
}

// RegisterSource registers a Go-native knowledge importer as an AUTO source,
// eligible for the startup "import all" pass. Built-in curated importers (the
// Claude memory importer) use this.
func (r *Runtime) RegisterSource(name string, importer plugin.KnowledgeImporter) {
	r.registerSource(name, importer, true)
}

// RegisterSourceManual registers a Go-native importer that is excluded from the
// startup auto-import and only runs when triggered explicitly by name. Used for
// privacy-sensitive sources (codex session transcripts). ADR-0011 决策 4.
func (r *Runtime) RegisterSourceManual(name string, importer plugin.KnowledgeImporter) {
	r.registerSource(name, importer, false)
}

func (r *Runtime) registerSource(name string, importer plugin.KnowledgeImporter, auto bool) {
	if importer == nil {
		return
	}
	r.mu.Lock()
	r.sources[name] = &sourceEntry{name: name, plugin: "builtin", importFn: importer.Import, auto: auto}
	r.mu.Unlock()
}
