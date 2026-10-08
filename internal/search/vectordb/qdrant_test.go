package vectordb

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type qStub struct {
	t          *testing.T
	auth       string
	lastCreate string
	lastUpsert string
	lastSearch string
}

func (s *qStub) serve(w http.ResponseWriter, r *http.Request) {
	s.auth = r.Header.Get("api-key")
	body, _ := io.ReadAll(r.Body)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/collections":
		writeJSON(w, map[string]any{"result": map[string]any{"collections": []any{}}})
	case r.Method == http.MethodPut && r.URL.Path == "/collections/reponest_vecs":
		s.lastCreate = string(body)
		writeJSON(w, map[string]any{"result": true, "status": "ok"})
	case r.Method == http.MethodDelete && r.URL.Path == "/collections/reponest_vecs":
		writeJSON(w, map[string]any{"result": true, "status": "ok"})
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/collections/reponest_vecs/points"):
		s.lastUpsert = string(body)
		writeJSON(w, map[string]any{"result": map[string]any{"status": "completed"}})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/points/search"):
		s.lastSearch = string(body)
		writeJSON(w, map[string]any{"result": []any{map[string]any{"id": 42, "score": 0.9}}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestQdrantRoundTrip(t *testing.T) {
	stub := &qStub{t: t}
	srv := httptest.NewServer(http.HandlerFunc(stub.serve))
	defer srv.Close()

	q, err := NewQdrant(srv.URL, "k-123", "reponest_vecs")
	if err != nil {
		t.Fatal(err)
	}
	if !q.Reachable() {
		t.Fatal("expected reachable")
	}
	if stub.auth != "k-123" {
		t.Fatalf("api-key header = %q", stub.auth)
	}
	if err := q.Ensure(3); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !strings.Contains(stub.lastCreate, `"size":3`) || !strings.Contains(stub.lastCreate, "Cosine") {
		t.Errorf("create body wrong: %s", stub.lastCreate)
	}
	if err := q.Upsert(7, []float32{1, 2, 3}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !strings.Contains(stub.lastUpsert, `"id":7`) {
		t.Errorf("upsert body missing id: %s", stub.lastUpsert)
	}
	ids, err := q.Search([]float32{1, 1, 1}, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !reflect.DeepEqual(ids, []int64{42}) {
		t.Fatalf("search ids = %v, want [42]", ids)
	}
}

func TestQdrantRejectsBadBase(t *testing.T) {
	if _, err := NewQdrant("localhost:6333", "", "x"); err == nil {
		t.Error("non-http base should error (forces local fallback)")
	}
}

func TestOpenFallsBackToLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Unreachable qdrant (server closed) -> local.
	srv.Close()
	got := Open(openMem(t), "qdrant", "http://127.0.0.1:1", "", "reponest_vecs")
	if got.Name() != "local-sqlite-vec" {
		t.Fatalf("unreachable qdrant should fall back to local, got %s", got.Name())
	}
	// Reachable qdrant -> qdrant.
	stub := &qStub{t: t}
	up := httptest.NewServer(http.HandlerFunc(stub.serve))
	defer up.Close()
	if got := Open(openMem(t), "qdrant", up.URL, "k", "reponest_vecs"); got.Name() != "qdrant" {
		t.Fatalf("reachable qdrant should be chosen, got %s", got.Name())
	}
	// Empty/local kind -> local.
	if got := Open(openMem(t), "", "", "", ""); got.Name() != "local-sqlite-vec" {
		t.Fatalf("default should be local, got %s", got.Name())
	}
}

func TestKindsAndUnknownFallsBack(t *testing.T) {
	ks := Kinds()
	if len(ks) < 2 || ks[0] != "local" {
		t.Fatalf("Kinds()=%v, want local first + at least qdrant", ks)
	}
	found := false
	for _, k := range ks {
		// chromem is a registered backend too (pure-Go embedded store, ADR-0013
		// candidate matrix); the registry list only grows, so assert by
		// membership rather than position.
		if k == "qdrant" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Kinds() missing qdrant: %v", ks)
	}
	// Unknown kind -> local, no panic.
	if got := Open(openMem(t), "bogus-store", "", "", ""); got.Name() != "local-sqlite-vec" {
		t.Fatalf("unknown kind should fall back to local, got %s", got.Name())
	}
}
