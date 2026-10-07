// Command server runs RepoNest as a headless HTTP service, exposing the shared
// internal/service business logic over a JSON API. It is the bridge that lets
// the DeepSeek Harness dsh-plugin (and any other HTTP client) reuse the exact
// same analysis code the desktop App uses, without duplicating logic.
//
// Usage:
//
//	go build -o reponest-server ./cmd/server
//	./reponest-server --port 18765
//	REPONEST_HTTP_PORT=18765 ./reponest-server
//
// (This is its own binary, not a subcommand: the desktop `reponest` executable at
// the repo root has no argument dispatch — it launches Wails and returns.)
//
// The server binds to 127.0.0.1 only — it is a local agent, not a public
// service. The dsh-plugin spawns this process and connects to the chosen port.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"repo-nest/internal/app"
	"repo-nest/internal/db"
	"repo-nest/internal/httpapi"
	"repo-nest/internal/platform"
	"repo-nest/internal/service"
	"repo-nest/internal/version"
)

// Server timeouts. ReadHeaderTimeout bounds how long a local process can hold
// a half-open connection before sending a request; without it, slowloris-style
// hangs accumulate unbounded goroutines. IdleTimeout reaps keep-alive
// connections a client stopped using (without it they live forever).
// WriteTimeout is generous — the longest synchronous paths (llms.txt
// generation, search) are second-scale, and long work like scans/stats
// refresh is dispatched to background goroutines and returns immediately.
const (
	headerTimeout = 10 * time.Second
	writeTimeout  = 60 * time.Second
	idleTimeout   = 120 * time.Second
)

func main() {
	platform.SetPrivateUmask() // owner-only files: DB sidecars, logs, exports

	port := flag.String("port", envOr("REPONEST_HTTP_PORT", "18765"), "HTTP port for the headless API (loopback only)")
	flag.Parse()

	log.Printf("RepoNest headless server %s starting on 127.0.0.1:%s", version.Version, *port)

	database, err := db.InitDB(platform.GetDbPath())
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	gitUser := platform.GetGitUserName()
	svc := service.New(database, gitUser)

	// Seed default scan roots like the desktop App and MCP entry points do
	// (idempotent via the scan_roots_seeded flag): without it a fresh headless
	// install has no roots and TriggerScan fails until the user configures
	// one by hand.
	svc.EnsureDefaultScanRoots()

	// Run the same startup sequence the desktop App runs. Without it the
	// knowledge-source importers are never registered, so GetKnowledgeSources
	// returns an empty list and TriggerKnowledgeImport fails with
	// `unknown knowledge source`. Startup is sync.Once-guarded and also honours
	// the auto_import config key, so headless clients get the same import
	// behaviour as the desktop app instead of a silently reduced feature set.
	svc.Startup()

	// Bind the same App object the desktop Wails layer exposes, so the browser
	// frontend can reach every capability via /api/rpc (see httpapi/rpc.go).
	mux := httpapi.New(svc, app.New(svc))
	srv := &http.Server{
		Addr:              "127.0.0.1:" + *port,
		Handler:           mux,
		ReadHeaderTimeout: headerTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Graceful shutdown on SIGINT/SIGTERM (the dsh-plugin stops the process
	// with SIGTERM): stop accepting, let in-flight handlers finish, then run
	// the service shutdown (cancels background work and waits for it) and
	// close the database. A SIGKILL has no such window and relies on SQLite's
	// WAL crash-safety, as documented in the service layer.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.ListenAndServe() }()

	log.Printf("RepoNest headless API listening at http://127.0.0.1:%s", *port)
	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	case <-ctx.Done():
		log.Printf("shutdown signal received; draining connections")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown: %v", err)
		}
	}

	svc.Shutdown()
	if err := svc.Close(); err != nil {
		log.Printf("close database: %v", err)
	}
	log.Printf("RepoNest headless server stopped")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
