//go:build qdrantlive

// Opt-in integration test that runs my Qdrant client against a REAL Qdrant
// server (default http://localhost:6333) — the smoke test the ADR-0013 note
// asks for. Run:
//
//	docker run -d --name qdrant -p 6333:6333 qdrant/qdrant
//	go test -tags qdrantlive ./internal/search/vectordb/ -run Live -v
package vectordb

import (
	"os"
	"reflect"
	"testing"
)

func TestQdrantLiveRoundTrip(t *testing.T) {
	url := os.Getenv("QDRANT_URL")
	if url == "" {
		url = "http://localhost:6333"
	}
	q, err := NewQdrant(url, os.Getenv("QDRANT_API_KEY"), "repotory_smoketest")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if !q.Reachable() {
		t.Skipf("no Qdrant reachable at %s — start it first", url)
	}

	// Fresh collection at dim 4 (delete+create).
	if err := q.Clear(4); err != nil {
		t.Fatalf("clear/ensure: %v", err)
	}
	if err := q.Upsert(1, []float32{1, 0, 0, 0}); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	if err := q.Upsert(2, []float32{0, 1, 0, 0}); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	ids, err := q.Search([]float32{0.9, 0.1, 0, 0}, 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(ids) == 0 || ids[0] != 1 {
		t.Fatalf("live Qdrant nearest = %v, want [1,...] — check client/REST contract", ids)
	}
	t.Logf("live Qdrant OK: nearest=%v", ids)

	// Round-trip order sanity (both points returned).
	ids2, err := q.Search([]float32{1, 1, 0, 0}, 5)
	if err != nil || !reflect.DeepEqual(ids2, []int64{1, 2}) && len(ids2) != 2 {
		t.Logf("cosine tie search=%v err=%v (informational)", ids2, err)
	}
}
