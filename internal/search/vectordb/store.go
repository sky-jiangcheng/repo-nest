package vectordb

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
)

// remoteConfig carries what a remote-store factory needs (ignored by local).
type remoteConfig struct {
	url        string
	apiKey     string
	collection string
}

// factory builds a store from config; a non-nil error (or an unreachable
// remote) makes Open fall back to local. Adding a backend = write a Store +
// Register("kind", factory) — no change to callers.
type factory func(database *sql.DB, rc remoteConfig) (Store, error)

var registry = map[string]factory{
	"qdrant": func(_ *sql.DB, rc remoteConfig) (Store, error) {
		q, err := NewQdrant(rc.url, rc.apiKey, rc.collection)
		if err != nil {
			return nil, err
		}
		if !q.Reachable() {
			return nil, fmt.Errorf("qdrant %s unreachable", q.Base)
		}
		return q, nil
	},
	// Future backends register here: "lancedb" (NOTE: CGO → breaks the
	// zero-CGO build constraint), "bleve" (pure-Go, also subsumes text).
	// See ADR-0013 candidate matrix.
	"weaviate": func(_ *sql.DB, rc remoteConfig) (Store, error) {
		w, err := NewWeaviate(rc.url, rc.apiKey, rc.collection)
		if err != nil {
			return nil, err
		}
		if !w.Reachable() {
			return nil, fmt.Errorf("weaviate %s unreachable", w.Base)
		}
		return w, nil
	},
	// chromem is an embedded pure-Go store, not a server: the "url" config
	// field carries its persistence DIRECTORY. It never fails a reachability
	// probe (there is nothing to reach), so a configured chromem always
	// opens — but an empty url means in-memory, which would silently drop
	// every embedding on restart. Refuse that configuration rather than
	// pretend it works.
	"chromem": func(_ *sql.DB, rc remoteConfig) (Store, error) {
		if strings.TrimSpace(rc.url) == "" {
			return nil, fmt.Errorf("chromem requires a persistence directory in vector_store_url (e.g. the app data dir); refusing an in-memory store that would lose every embedding on restart")
		}
		return NewChromem(rc.url, rc.collection)
	},
}

// Register adds a named store backend to the selectable set (used by
// future adapters; local is always implicit).
func Register(kind string, f factory) { registry[strings.ToLower(kind)] = f }

// Kinds lists selectable vector-store backends (for help / validation / UI).
// "local" is always first: it is the default, not a registry entry, and the
// sort below is scoped to the registry keys only. (Sorting the whole list
// would let a backend whose name sorts before "local" — chromem does —
// silently displace the default from the head of the list, and UI copy that
// reads Kinds()[0] would stop naming the default.)
func Kinds() []string {
	out := make([]string, 0, len(registry)+1)
	out = append(out, "local")
	rest := make([]string, 0, len(registry))
	for k := range registry {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// Open resolves the configured store. local is the default; an empty/"local",
// an unknown kind, or a remote that errors/is-unreachable all fall back to the
// local sqlite-vec store (ADR-0013: never make search return less than before).
func Open(database *sql.DB, kind, url, apiKey, collection string) Store {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" || kind == "local" {
		return NewLocal(database)
	}
	f, ok := registry[kind]
	if !ok {
		log.Printf("vectordb: unknown vector_store %q (available: %v); using local sqlite-vec", kind, Kinds())
		return NewLocal(database)
	}
	st, err := f(database, remoteConfig{url, apiKey, collection})
	if err != nil {
		log.Printf("vectordb: vector_store %q unavailable (%v); using local sqlite-vec", kind, err)
		return NewLocal(database)
	}
	return st
}
