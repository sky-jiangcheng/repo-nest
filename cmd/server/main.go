// Command server runs RepoNest as a headless HTTP service, exposing the shared
// internal/service business logic over a JSON API. It is the bridge that lets
// the DeepSeek Harness dsh-plugin (and any other HTTP client) reuse the exact
// same analysis code the desktop App uses, without duplicating logic.
//
// Usage:
//
//	reponest server [--port 18765]
//	REPONEST_HTTP_PORT=18765 reponest server
//
// The server binds to 127.0.0.1 only — it is a local agent, not a public
// service. The dsh-plugin spawns this process and connects to the chosen port.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"repo-nest/internal/app"
	"repo-nest/internal/db"
	"repo-nest/internal/httpapi"
	"repo-nest/internal/platform"
	"repo-nest/internal/service"
	"repo-nest/internal/version"
)

// headerTimeout bounds request-header reads on the loopback API; every
// legitimate local client sends its headers in one segment.
const headerTimeout = 10 * time.Second

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
	// ReadHeaderTimeout bounds how long a local process can hold a half-open
	// connection before sending a request; without it, slowloris-style hangs
	// accumulate unbounded goroutines.
	srv := &http.Server{
		Addr:              "127.0.0.1:" + *port,
		Handler:           mux,
		ReadHeaderTimeout: headerTimeout,
	}

	// Best-effort cleanup on exit (e.g. when the plugin stops the process).
	defer func() { _ = svc.Close() }()

	log.Printf("RepoNest headless API listening at http://127.0.0.1:%s", *port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
