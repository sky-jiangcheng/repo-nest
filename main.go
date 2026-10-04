package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"

	"repo-nest/internal/app"
	"repo-nest/internal/db"
	"repo-nest/internal/platform"
	"repo-nest/internal/service"
	"repo-nest/internal/version"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:web/dist
var assets embed.FS

func main() {
	platform.SetPrivateUmask() // owner-only files: DB sidecars, logs, exports
	log.Printf("RepoNest %s starting...", version.Version)

	// Open database
	database, err := db.InitDB(platform.GetDbPath())
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Detect git user
	gitUser := platform.GetGitUserName()
	log.Printf("Git user for personal stats: %s", gitUser)

	// Create the service core and the thin Wails binding layer over it. Scan
	// roots are seeded through the service (shared with the MCP server) so the
	// first-run behaviour is identical on both entry points.
	svc := service.New(database, gitUser)
	svc.EnsureDefaultScanRoots()
	a := app.New(svc)

	// Launch Wails
	err = wails.Run(&options.App{
		Title:     "RepoNest: Local Git Knowledge Base",
		Width:     1280,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: spaFallback{},
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Security headers on every response.
					// script-src has no 'unsafe-inline': the built index.html
					// references a single external module bundle, and Wails
					// injects its runtime as a native user script (WKUserScript
					// on macOS, AddScriptToExecuteOnDocumentCreated on
					// Windows), which page CSP does not govern. worker-src
					// blob: covers Mermaid's ELK layout workers. connect-src
					// allows no remote host: the app makes no remote API calls,
					// and neither ws: nor wss: is listed because the shipped
					// frontend opens no WebSocket (vite HMR is a dev-server
					// concern and never runs behind this handler). Two channels
					// remain open by choice: img-src https: (remote note
					// images) and 'self' fetches — both can exfiltrate data to
					// a host the app already talks to, which is none, so an
					// injected script's practical exfil routes are the same
					// ones a <img src="https://..."> tag has; CSP narrows the
					// blast radius, it is not a hard exfil wall. unsafe-eval is
					// intentionally omitted; if dynamic eval is needed,
					// refactor to use explicit Function() calls with a nonce
					// instead.
					w.Header().Set("Content-Security-Policy",
						"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self'; connect-src 'self'; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
					w.Header().Set("X-Content-Type-Options", "nosniff")
					w.Header().Set("X-Frame-Options", "DENY")
					w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
					w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), acceleration=()")
					next.ServeHTTP(w, r)
				})
			},
		},
		OnStartup:  a.Startup,
		OnShutdown: a.Shutdown,
		Bind: []interface{}{
			a,
		},
	})
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}

// spaFallback serves index.html for any GET request the embedded Assets could
// not resolve. The browser build may use real SPA paths, so a direct load or
// refresh of a deep link must receive the app shell. The desktop WebView uses
// hash navigation to avoid custom-origin history limitations.
type spaFallback struct{}

// ServeHTTP serves the SPA shell for any path not found in the embedded assets.
func (spaFallback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data, err := assets.ReadFile("web/dist/index.html")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func init() {
	// Ensure PATH includes common binary directories (git may not be in PATH
	// when launched from Finder).
	ensurePath()
	// Redirect logs to a file so crashes can be diagnosed when the app is
	// launched outside a terminal.
	setupLogging()
}

func ensurePath() {
	// The motivating case is macOS: an app launched from Finder inherits a
	// minimal PATH, so git is not found. The standard dirs are Unix paths —
	// prepending them on Windows would just add permanent junk entries.
	if runtime.GOOS != "darwin" {
		return
	}
	path := os.Getenv("PATH")
	if path == "" {
		os.Setenv("PATH", "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
		return
	}
	// Prepend standard directories if not already present.
	standardDirs := []string{"/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"}
	seen := make(map[string]bool)
	for _, dir := range strings.Split(path, string(os.PathListSeparator)) {
		seen[dir] = true
	}
	var toAdd []string
	for _, dir := range standardDirs {
		if !seen[dir] {
			toAdd = append(toAdd, dir)
		}
	}
	if len(toAdd) > 0 {
		os.Setenv("PATH", strings.Join(toAdd, string(os.PathListSeparator))+string(os.PathListSeparator)+path)
	}
}

func setupLogging() {
	logFile := platform.GetLogPath()
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err == nil {
		log.SetOutput(f)
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("=== RepoNest log started ===")
	log.Printf("log file: %s", logFile)
	log.Printf("PATH=%s", os.Getenv("PATH"))
}
