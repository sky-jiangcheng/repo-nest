// Package runtime implements the in-process plugin runtime for RepoNest.
//
// Plugins are Go scripts loaded via the yaegi interpreter (see ADR 0002 and
// issue #33). Each plugin lives in its own directory under the plugins dir
// (platform.GetPluginsDir) and must contain a plugin.go file exporting:
//
//	func Name() string                                     // required
//	func Init(ctx *plugin.Context) error                   // required
//	func Source() string                                   // optional, defaults to Name
//	func Import(ctx *plugin.Context) ([]plugin.ImportDoc, error) // optional knowledge source
//
// Scripts import "repo-nest/internal/core/plugin" for the host-provided types.
// All plugin calls are wrapped in recover() so a panicking plugin can never
// crash the host process.
package runtime

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"repo-nest/internal/core/plugin"
	"repo-nest/internal/db"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// PluginStatus describes the load result of one plugin, surfaced on the
// settings page so failures are visible instead of silent.
type PluginStatus struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Loaded bool   `json:"loaded"`
	Err    string `json:"error,omitempty"`
}

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
	// (ImportAll). Curated sources (claude memory, script-plugin importers) are
	// auto; privacy-sensitive transcript sources (codex sessions) are opt-in
	// and only run when triggered explicitly by name. See ADR-0011 决策 4.
	auto bool
}

// Runtime loads and supervises plugins. It is safe for concurrent use.
type Runtime struct {
	mu       sync.Mutex
	db       *sql.DB
	plugins  []PluginStatus
	handlers map[string][]plugin.EventHandler
	sources  map[string]*sourceEntry
	// importMu serializes TriggerImport so concurrent invocations of the same
	// source (e.g. UI button + startup auto-import) cannot double-import.
	importMu sync.Mutex
}

// New creates a Runtime bound to the given database handle.
func New(database *sql.DB) *Runtime {
	return &Runtime{
		db:       database,
		handlers: make(map[string][]plugin.EventHandler),
		sources:  make(map[string]*sourceEntry),
	}
}

// Load scans dir for plugin directories (dir/*/) and loads each one.
// A missing directory is not an error: no plugins are loaded.
func (r *Runtime) Load(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("plugin runtime: plugins dir %s unavailable (%v); skipping", dir, err)
		return
	}

	var pending []string
	for _, e := range entries {
		if e.IsDir() {
			pending = append(pending, filepath.Join(dir, e.Name()))
		}
	}

	r.mu.Lock()
	r.plugins = nil
	r.handlers = make(map[string][]plugin.EventHandler)
	r.sources = make(map[string]*sourceEntry)
	r.mu.Unlock()

	for _, p := range pending {
		r.loadPlugin(p)
	}
	// Loaded plugins run interpreted Go inside this process with full host
	// authority (direct database access via plugin.Context). The loaded paths
	// are logged so it is always traceable what code was admitted, from where.
	log.Printf("plugin runtime: loaded %d plugin(s) from %s — plugins run with full host privileges (DB access); only install plugins you trust", len(r.plugins), dir)
	for _, p := range r.plugins {
		if p.Loaded {
			log.Printf("plugin runtime:   + %s (%s)", p.Name, p.Path)
		}
	}
}

// loadPlugin loads a single plugin directory. Any panic during eval or init
// is recovered and recorded instead of propagating.
func (r *Runtime) loadPlugin(dir string) {
	status := PluginStatus{Path: dir}

	defer func() {
		if rec := recover(); rec != nil {
			status.Loaded = false
			status.Err = fmt.Sprintf("panic: %v", rec)
			log.Printf("plugin runtime: %s panicked during load: %v", dir, rec)
		}
		r.mu.Lock()
		r.plugins = append(r.plugins, status)
		r.mu.Unlock()
	}()

	src, err := os.ReadFile(filepath.Join(dir, "plugin.go"))
	if err != nil {
		status.Err = fmt.Sprintf("no plugin.go: %v", err)
		return
	}

	script, err := compileScript(string(src))
	if err != nil {
		status.Err = err.Error()
		return
	}

	nameFn, err := script.funcValue("main.Name")
	if err != nil {
		status.Err = fmt.Sprintf("Name: %v", err)
		return
	}
	name, ok := nameFn.Interface().(func() string)
	if !ok {
		status.Err = "Name must have signature func() string"
		return
	}
	status.Name = name()

	initFn, err := script.funcValue("main.Init")
	if err != nil {
		status.Err = fmt.Sprintf("Init: %v", err)
		return
	}
	init, ok := initFn.Interface().(func(*Context) error)
	if !ok {
		status.Err = "Init must have signature func(*plugin.Context) error"
		return
	}

	ctx := r.newContext()
	if err := init(ctx); err != nil {
		// Roll back the handlers Init registered before failing: a failed
		// plugin must not keep receiving events.
		r.unregisterHandlers(ctx)
		status.Err = fmt.Sprintf("Init: %v", err)
		return
	}

	// Optional knowledge source: Source() + Import(ctx).
	source := status.Name
	if sv, err := script.funcValue("main.Source"); err == nil {
		if sf, ok := sv.Interface().(func() string); ok {
			source = sf()
		}
	}
	if iv, err := script.funcValue("main.Import"); err == nil {
		imp, ok := iv.Interface().(func(*Context) ([]plugin.ImportDoc, error))
		if !ok {
			// Init already succeeded, but the plugin as a whole is a load
			// failure — roll its handlers back too.
			r.unregisterHandlers(ctx)
			status.Err = "Import must have signature func(*plugin.Context) ([]plugin.ImportDoc, error)"
			return
		}
		importFn := func() ([]plugin.ImportDoc, error) { return imp(ctx) }
		r.mu.Lock()
		r.sources[source] = &sourceEntry{name: source, plugin: status.Name, importFn: importFn, auto: true}
		r.mu.Unlock()
		log.Printf("plugin runtime: plugin %s registered knowledge source %q", status.Name, source)
	}

	status.Loaded = true
	log.Printf("plugin runtime: loaded plugin %q from %s", status.Name, dir)
}

// PluginStatuses returns the current load status of every plugin directory.
func (r *Runtime) PluginStatuses() []PluginStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]PluginStatus, len(r.plugins))
	copy(out, r.plugins)
	return out
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

// Emit delivers an event to every registered handler. A panicking handler is
// recovered and logged; the host never crashes.
func (r *Runtime) Emit(name string, data any) {
	r.mu.Lock()
	handlers := append([]plugin.EventHandler(nil), r.handlers[name]...)
	r.mu.Unlock()

	ev := plugin.Event{Name: name, Data: data}
	for _, h := range handlers {
		r.safeCall(func() error { return h(ev) }, fmt.Sprintf("event %q", name))
	}
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

// safeCall runs fn, recovering any panic.
func (r *Runtime) safeCall(fn func() error, what string) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("plugin runtime: panic in %s: %v", what, rec)
		}
	}()
	if err := fn(); err != nil {
		log.Printf("plugin runtime: handler %s returned error: %v", what, err)
	}
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

// Context implements plugin.PluginContext for scripts. Scripts receive it as
// *plugin.Context and call On to subscribe to events.
type Context struct {
	db *sql.DB
	rt *Runtime
	// initLog, when non-nil, records every On() registration so a failed
	// plugin Init can be rolled back — without it, a plugin whose Init
	// registered handlers and then returned an error stayed marked as load-
	// failed while its handlers kept receiving events.
	initLog []handlerTicket
}

// handlerTicket remembers one On() registration: the event name and the
// handler-slice length before the append.
type handlerTicket struct {
	event string
	index int
}

func (r *Runtime) newContext() *Context {
	return &Context{db: r.db, rt: r}
}

// DB exposes the underlying SQLite handle.
func (c *Context) DB() *sql.DB { return c.db }

// On registers a handler for a runtime event.
func (c *Context) On(name string, handler plugin.EventHandler) {
	c.rt.mu.Lock()
	if c.initLog != nil {
		c.initLog = append(c.initLog, handlerTicket{event: name, index: len(c.rt.handlers[name])})
	}
	c.rt.handlers[name] = append(c.rt.handlers[name], handler)
	c.rt.mu.Unlock()
}

// unregisterHandlers rolls back every On() registration recorded on the
// context's init log (used when a plugin's Init fails mid-way).
func (r *Runtime) unregisterHandlers(ctx *Context) {
	if ctx.initLog == nil {
		return
	}
	r.mu.Lock()
	for i := len(ctx.initLog) - 1; i >= 0; i-- {
		t := ctx.initLog[i]
		if hs, ok := r.handlers[t.event]; ok && t.index <= len(hs) {
			hs = hs[:t.index]
			if len(hs) == 0 {
				delete(r.handlers, t.event)
			} else {
				r.handlers[t.event] = hs
			}
		}
	}
	r.mu.Unlock()
	ctx.initLog = nil
}

// RegisterKnowledgeSource registers a Go-native importer (kept for interface
// compatibility; script plugins export Import instead).
func (c *Context) RegisterKnowledgeSource(name string, importer plugin.KnowledgeImporter) {
	c.rt.RegisterSource(name, importer)
}

// RegisterSource registers a Go-native knowledge importer as an AUTO source,
// eligible for the startup "import all" pass. Built-in curated importers (the
// Claude memory importer) and script-plugin sources use this.
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

// ---------------------------------------------------------------------------
// yaegi script loader
// (merged from loader.go — C11 方案A: reduce runtime-package file fragmentation)
// ---------------------------------------------------------------------------

// exportedTypes is the set of host symbols exposed to plugin scripts via the
// import path "repo-nest/internal/core/plugin". Scripts use plugin.Context and
// plugin.ImportDoc.
var exportedTypes = interp.Exports{
	"repo-nest/internal/core/plugin/plugin": {
		"Context":   reflect.ValueOf((*Context)(nil)),
		"Event":     reflect.ValueOf(plugin.Event{}),
		"ImportDoc": reflect.ValueOf(plugin.ImportDoc{}),
	},
}

// safeSymbolPkgs is the allowlist of stdlib packages a plugin may import.
//
// A plugin already holds full host authority by design: it receives
// plugin.Context with direct database access, and loading it is an explicit
// act of trust in its source. But the interpreter must not ALSO hand out the
// packages that reach past RepoNest entirely — os/os/exec/syscall/unsafe
// escape to the filesystem and process table, net*/net/http reach the
// network — because then a single dropped-in plugin.go becomes arbitrary
// code execution in the app process with no product-level gate.
// Extend deliberately, never wholesale.
var safeSymbolPkgs = map[string]bool{
	"bufio":           true,
	"bytes":           true,
	"cmp":             true,
	"context":         true,
	"encoding/base64": true,
	"encoding/csv":    true,
	"encoding/hex":    true,
	"encoding/json":   true,
	"encoding/utf8":   true,
	"errors":          true,
	"fmt":             true,
	"hash":            true,
	"hash/crc32":      true,
	"hash/crc64":      true,
	"hash/fnv":        true,
	"io":              true,
	"maps":            true,
	"math":            true,
	"math/bits":       true,
	"math/rand":       true,
	"net/url":         true,
	"path":            true,
	"path/filepath":   true,
	"regexp":          true,
	"slices":          true,
	"sort":            true,
	"strconv":         true,
	"strings":         true,
	"sync":            true,
	"sync/atomic":     true,
	"text/tabwriter":  true,
	"text/template":   true,
	"time":            true,
	"unicode":         true,
	"unicode/utf16":   true,
	"unicode/utf8":    true,
}

// safeSymbols filters stdlib.Symbols down to safeSymbolPkgs. yaegi keys its
// stdlib entries as "<import path>/<package name>" ("fmt/fmt", "os/exec/exec"),
// so the trailing element is stripped to get the path a plugin would import.
// The package drop is silent — a plugin importing an excluded package fails
// at Eval time with an "unable to find source" error, which loadPlugin
// records as a normal per-plugin load failure.
func safeSymbols() map[string]map[string]reflect.Value {
	out := make(map[string]map[string]reflect.Value, len(safeSymbolPkgs))
	for key, symbols := range stdlib.Symbols {
		i := strings.LastIndexByte(key, '/')
		if i < 0 || !safeSymbolPkgs[key[:i]] {
			// No slash: the "." root entry and similar non-package keys.
			continue
		}
		out[key] = symbols
	}
	return out
}

// script wraps a yaegi interpreter bound to one plugin source file.
type script struct {
	i *interp.Interpreter
}

// compileScript evaluates a plugin source file and prepares it for symbol
// lookup. Compilation errors are returned verbatim.
func compileScript(src string) (*script, error) {
	i := interp.New(interp.Options{})
	i.Use(safeSymbols())
	i.Use(exportedTypes)

	if _, err := i.Eval(src); err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	return &script{i: i}, nil
}

// funcValue looks up a package-level symbol such as "main.Name".
func (s *script) funcValue(sym string) (reflect.Value, error) {
	v, err := s.i.Eval(sym)
	if err != nil {
		return reflect.Value{}, err
	}
	if !v.IsValid() || v.Kind() != reflect.Func {
		return reflect.Value{}, fmt.Errorf("%s is not a function", sym)
	}
	return v, nil
}
