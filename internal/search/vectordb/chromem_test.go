package vectordb

import (
	"path/filepath"
	"testing"
)

// chromem is an embedded store: no server to stub, but the contract with the
// rest of the package (Store interface, registry, fallback rules) is exactly
// what these tests pin. The persistence round-trip matters most — a store
// that forgets its vectors between process runs is worse than no store.

func TestChromemRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chromem")
	c, err := NewChromem(dir, "reponest_vecs")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Name(); got != "chromem-go" {
		t.Errorf("Name() = %q, want the persistent label", got)
	}

	if err := c.Ensure(4); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	vec := []float32{0.1, 0.2, 0.3, 0.4}
	if err := c.Upsert(42, vec); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Re-upsert same id: must replace, not duplicate or error.
	vec2 := []float32{0.4, 0.3, 0.2, 0.1}
	if err := c.Upsert(42, vec2); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if err := c.Upsert(7, []float32{0.9, 0.9, 0.0, 0.0}); err != nil {
		t.Fatalf("upsert 7: %v", err)
	}

	hits, err := c.Search(vec2, 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 || hits[0] != 42 {
		t.Errorf("nearest to vec2 should be note 42, got %v", hits)
	}

	// Delete is idempotent and really removes recall.
	if err := c.Delete(42); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := c.Delete(42); err != nil {
		t.Fatalf("delete twice must stay nil: %v", err)
	}
	hits, err = c.Search(vec2, 5)
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	for _, id := range hits {
		if id == 42 {
			t.Errorf("deleted note 42 still recalled: %v", hits)
		}
	}
}

func TestChromemPersistsAcrossReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chromem")

	c1, err := NewChromem(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c1.Ensure(3); err != nil {
		t.Fatal(err)
	}
	if err := c1.Upsert(5, []float32{1, 0, 0}); err != nil {
		t.Fatal(err)
	}

	// A second store over the same directory must see the vector — this is
	// the whole point of the persistent mode.
	c2, err := NewChromem(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	hits, err := c2.Search([]float32{1, 0, 0}, 1)
	if err != nil {
		t.Fatalf("search after reopen: %v", err)
	}
	if len(hits) != 1 || hits[0] != 5 {
		t.Errorf("expected note 5 to survive the reopen, got %v", hits)
	}
}

func TestChromemClearResetsButKeepsUsable(t *testing.T) {
	c, err := NewChromem("", "reponest_vecs") // in-memory variant
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Name(); got != "chromem-go (memory)" {
		t.Errorf("in-memory Name() = %q", got)
	}
	if err := c.Ensure(2); err != nil {
		t.Fatal(err)
	}
	if err := c.Upsert(1, []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := c.Clear(2); err != nil {
		t.Fatalf("clear: %v", err)
	}
	hits, err := c.Search([]float32{1, 0}, 5)
	if err != nil {
		t.Fatalf("search after clear: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("clear left vectors behind: %v", hits)
	}
	// The store must still accept writes after a clear (re-embed path).
	if err := c.Upsert(2, []float32{0, 1}); err != nil {
		t.Fatalf("upsert after clear: %v", err)
	}
}

func TestChromemSearchClampsLimitToCollectionSize(t *testing.T) {
	c, err := NewChromem("", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ensure(2); err != nil {
		t.Fatal(err)
	}
	if err := c.Upsert(1, []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	// chromem errors when nResults > collection size; our contract clamps.
	hits, err := c.Search([]float32{1, 0}, 10)
	if err != nil {
		t.Fatalf("over-limit search must clamp, not error: %v", err)
	}
	if len(hits) != 1 || hits[0] != 1 {
		t.Errorf("got %v, want [1]", hits)
	}
}

func TestChromemValidation(t *testing.T) {
	if _, err := NewChromem("relative/path", ""); err == nil {
		t.Error("relative persistence dir must be rejected: it would resolve against whatever cwd the process happens to have")
	}
	c, err := NewChromem("", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ensure(0); err == nil {
		t.Error("Ensure(0) must be rejected")
	}
	if err := c.Clear(0); err == nil {
		t.Error("Clear(0) must be rejected")
	}
	if err := c.Upsert(1, nil); err == nil {
		t.Error("empty vector must be rejected")
	}
	if _, err := c.Search([]float32{1}, 0); err == nil {
		t.Error("limit 0 must be rejected")
	}
}

// Registry wiring: chromem opens without a reachability probe (embedded), but
// an empty vector_store_url must fall back to local instead of losing every
// embedding on the next restart.
func TestChromemRegistryWiring(t *testing.T) {	// Unknown-URL chromem via Open: a real temp dir must yield the chromem store.
	dir := filepath.Join(t.TempDir(), "chromem")
	st := Open(nil, "chromem", dir, "", "")
	if st.Name() != "chromem-go" {
		t.Errorf("Open(chromem, dir) = %q, want chromem-go", st.Name())
	}
	if err := st.Ensure(2); err != nil {
		t.Fatalf("ensure via Open: %v", err)
	}
	if err := st.Upsert(9, []float32{0.5, 0.5}); err != nil {
		t.Fatalf("upsert via Open: %v", err)
	}

	// No directory configured: the factory refuses, Open falls back to local.
	fallback := Open(nil, "chromem", "", "", "")
	if fallback.Name() != "local-sqlite-vec" {
		t.Errorf("empty dir must fall back to local, got %q", fallback.Name())
	}
}
