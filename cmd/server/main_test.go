package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// newTestWebHandler builds a wrapper over a sentinel API handler and a temp
// dist containing index.html and assets/app.js. Returns the handler and a
// marker the API handler sets when it was reached.
func newTestWebHandler(t *testing.T) (http.Handler, *bool) {
	t.Helper()
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html>spa-index</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "assets", "app.js"), []byte("console.log('app')"), 0o644); err != nil {
		t.Fatal(err)
	}
	apiReached := new(bool)
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*apiReached = true
		w.WriteHeader(http.StatusTeapot)
	})
	return newWebHandler(dist, api), apiReached
}

func do(t *testing.T, h http.Handler, target string) (int, string, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes()
}

// doRawPath feeds the handler a request whose URL.Path already contains dot
// segments, bypassing URL parsing (both net/http and httptest reject ".."
// before a handler ever runs — this pins the handler's own defence-in-depth).
func doRawPath(t *testing.T, h http.Handler, rawPath string) (int, string, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, &http.Request{Method: http.MethodGet, URL: &url.URL{Path: rawPath}, Host: "127.0.0.1"})
	return rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes()
}

// TestNewWebHandlerRouting pins the three-way decision the --web-dist wrapper
// makes for every request: API paths reach the mux, real files are served,
// page routes fall back to index.html, and missing assets are genuine 404s.
func TestNewWebHandlerRouting(t *testing.T) {
	h, apiReached := newTestWebHandler(t)

	cases := []struct {
		name       string
		target     string
		wantStatus int
		wantCT     string // "" = not asserted
		wantInBody string
		wantAPI    bool
	}{
		{"rpc bridge reaches mux", "/api/rpc", http.StatusTeapot, "", "", true},
		{"project subtree reaches mux", "/api/project/25/stats?date=2026-10-14", http.StatusTeapot, "", "", true},
		{"health reaches mux", "/health", http.StatusTeapot, "", "", true},
		{"ask-stream reaches mux", "/api/ai/ask-stream", http.StatusTeapot, "", "", true},
		{"root serves index", "/", http.StatusOK, "text/html", "spa-index", false},
		{"deep page route falls back to index", "/project/25", http.StatusOK, "text/html", "spa-index", false},
		{"unknown page route falls back to index", "/no-such-route", http.StatusOK, "text/html", "spa-index", false},
		{"real asset is served", "/assets/app.js", http.StatusOK, "javascript", "console.log", false},
		{"missing asset is a real 404", "/assets/missing.js", http.StatusNotFound, "text/plain", "", false},
		{"missing css under page route is 404 not index", "/project/foo.css", http.StatusNotFound, "text/plain", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			*apiReached = false
			code, ct, body := do(t, h, c.target)
			if code != c.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", code, c.wantStatus, body)
			}
			if c.wantCT != "" && !strings.Contains(ct, c.wantCT) {
				t.Fatalf("content-type = %q, want it to contain %q", ct, c.wantCT)
			}
			if c.wantInBody != "" && !strings.Contains(string(body), c.wantInBody) {
				t.Fatalf("body %q does not contain %q", body, c.wantInBody)
			}
			if *apiReached != c.wantAPI {
				t.Fatalf("api handler reached = %v, want %v", *apiReached, c.wantAPI)
			}
		})
	}

	// Defence-in-depth for dot segments that already reached the handler
	// (real servers reject them earlier): the wrapper's Clean+prefix check
	// confines them inside dist, and http.ServeFile adds its own second gate
	// that answers 400 for any path containing "..". Either way the request
	// must never escape to the filesystem.
	t.Run("traversal never escapes dist", func(t *testing.T) {
		*apiReached = false
		code, _, body := doRawPath(t, h, "/../../etc/passwd")
		if code != http.StatusBadRequest && code != http.StatusOK {
			t.Fatalf("got status %d, want 400 (ServeFile gate) or 200 (index fallback)", code)
		}
		if strings.Contains(string(body), "root:") {
			t.Fatal("traversal leaked filesystem content")
		}
		if *apiReached {
			t.Fatal("traversal must not reach the API handler")
		}
	})
}

// TestNewWebHandlerEmptyDistIsAPIOnly pins the production behaviour: with no
// --web-dist, every path goes to the mux and nothing is served statically.
func TestNewWebHandlerEmptyDistIsAPIOnly(t *testing.T) {
	apiReached := new(bool)
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { *apiReached = true })
	h := newWebHandler("", api)

	do(t, h, "/anything/at/all")
	if !*apiReached {
		t.Fatal("empty webDist must pass every path through to the API handler")
	}
}

// TestIsAPIPathCoversHTTPAPIRoutes pins the cross-package invariant stated in
// isAPIPath's comment: every pattern registered on httpapi's mux must be
// claimed by isAPIPath. Adding a route in internal/httpapi — especially one
// outside /api — without updating the wrapper now fails here instead of
// silently returning index.html under --web-dist.
func TestIsAPIPathCoversHTTPAPIRoutes(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "internal", "httpapi", "server.go"))
	if err != nil {
		t.Fatalf("read httpapi server.go: %v", err)
	}
	re := regexp.MustCompile(`mux\.HandleFunc\("([^"]+)"`)
	matches := re.FindAllStringSubmatch(string(src), -1)
	if len(matches) == 0 {
		t.Fatal("no mux.HandleFunc patterns found — the extractor is stale, update it")
	}
	for _, m := range matches {
		pattern := m[1]
		if !isAPIPath(pattern) {
			t.Errorf("httpapi registers %q but isAPIPath does not claim it — a --web-dist run would swallow it into the SPA fallback", pattern)
		}
	}
}
