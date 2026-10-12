//go:build weavialive

// Opt-in real-server smoke test for the Weaviate backend (ADR-0013). Run:
//
//	docker run -d --name weaviate -p 8080:8080 \
//	  -e AUTH_ANONYMOUS_ACCESS_ENABLED=true -e PERSISTENCE_DATA_PATH=/var/lib/weaviate \
//	  -e DEFAULT_VECTORIZER_MODULE=none semitechnologies/weaviate:latest
//	go test -tags weavialive ./internal/search/vectordb/ -run WeaviateLive -v
package vectordb

import (
	"os"
	"testing"
)

func TestWeaviateLiveRoundTrip(t *testing.T) {
	url := os.Getenv("WEAVIATE_URL")
	if url == "" {
		url = "http://localhost:8080"
	}
	w, err := NewWeaviate(url, os.Getenv("WEAVIATE_API_KEY"), "repotory_smoketest")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if !w.Reachable() {
		t.Skipf("no Weaviate reachable at %s — start it first", url)
	}
	if err := w.Clear(4); err != nil { // fresh collection
		t.Fatalf("clear: %v", err)
	}
	if err := w.Upsert(1, []float32{1, 0, 0, 0}); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	if err := w.Upsert(2, []float32{0, 1, 0, 0}); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	ids, err := w.Search([]float32{0.9, 0.1, 0, 0}, 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(ids) == 0 || ids[0] != 1 {
		t.Fatalf("live Weaviate nearest = %v, want [1,...]", ids)
	}
	t.Logf("live Weaviate OK: nearest=%v (class %s)", ids, w.Class)
}
