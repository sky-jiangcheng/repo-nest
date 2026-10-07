package service

import (
	"errors"
	"testing"

	"repo-nest/internal/db"
	"repo-nest/internal/domain"
)

// downStore stands in for a vector backend that stopped answering: every
// operation errors. It is injected straight into the memo slot, which the
// previous code had no way to do — before the cache, a dead remote was
// re-probed on every query (and fell back by accident); with a memo it has to
// be dropped explicitly, and that is what these tests pin down.
type downStore struct{}

var errStoreDown = errors.New("vector store is down")

func (downStore) Name() string                  { return "down" }
func (downStore) Ensure(int) error              { return errStoreDown }
func (downStore) Clear(int) error               { return errStoreDown }
func (downStore) Upsert(int64, []float32) error { return errStoreDown }
func (downStore) Delete(int64) error            { return errStoreDown }
func (downStore) Search([]float32, int) ([]int64, error) {
	return nil, errStoreDown
}

// configureStubEmbeddings turns on semantic search against a fake endpoint, so
// fuseSemantic / RebuildEmbeddings get as far as the store.
func configureStubEmbeddings(t *testing.T, svc *Service) {
	t.Helper()
	srv := stubEmbedServer(t)
	t.Cleanup(srv.Close)
	for k, v := range map[string]string{
		"embedding_base_url": srv.URL + "/v1",
		"embedding_model":    "stub",
		"embedding_dim":      "2",
		"semantic_search":    "1",
	} {
		if err := svc.UpdateConfig(k, v); err != nil {
			t.Fatalf("config %s: %v", k, err)
		}
	}
}

// The whole point of the memo: resolving the store costs four config reads plus,
// for a remote backend, an HTTP reachability probe. Per search query that probe
// was the expensive part (TODO M3 遗留项).
func TestVectorStoreIsMemoised(t *testing.T) {
	svc, _ := setupService(t)

	first := svc.vectorStore()
	if first == nil {
		t.Fatal("vectorStore() = nil, want a resolved store (local fallback)")
	}
	for i := 0; i < 5; i++ {
		if got := svc.vectorStore(); got != first {
			t.Fatalf("call %d returned a different store (%v vs %v): memo not in effect, so a remote store would be re-probed per query", i, got, first)
		}
	}

	svc.invalidateVectorStore()
	if got := svc.vectorStore(); got == first {
		t.Fatal("store survived invalidateVectorStore(): invalidation is a no-op")
	}
}

// A reconfigured endpoint must take effect without a restart — otherwise the
// memo turns a settings change into a silently ignored input.
func TestVectorStoreConfigWritesDropMemo(t *testing.T) {
	svc, _ := setupService(t)

	for _, key := range []string{
		"vector_store", "vector_store_url", "vector_store_api_key", "vector_store_collection",
	} {
		if svc.vecStore.store != nil {
			t.Fatalf("pre-condition: memo already populated before probing %s", key)
		}
		if svc.vectorStore() == nil || svc.vecStore.store == nil {
			t.Fatalf("pre-condition: resolving the store left the memo empty, so %s has nothing to invalidate", key)
		}
		if err := svc.UpdateConfig(key, "probe"); err != nil {
			t.Fatalf("UpdateConfig(%s): %v", key, err)
		}
		if svc.vecStore.store != nil {
			t.Errorf("UpdateConfig(%s) left the memo populated; the new setting would not take effect until restart", key)
		}
	}

	// An unrelated key must NOT drop it, or the cache would be defeated by every
	// settings write (git_author and the embedding_* keys are written together
	// with semantic_search whenever the user touches the plugin tab).
	kept := svc.vectorStore()
	if err := svc.UpdateConfig("embedding_model", "other-model"); err != nil {
		t.Fatalf("UpdateConfig(embedding_model): %v", err)
	}
	if got := svc.vectorStore(); got != kept {
		t.Error("UpdateConfig on a non-vector_store key dropped the memo")
	}
}

// Query path: a store that errors must degrade to lexical results AND lose its
// memo slot, so the next query re-resolves (and falls back to local) instead of
// reusing a dead handle forever.
func TestFuseSemanticDropsDeadStore(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "cacheproj", "/tmp/cacheproj")
	if _, err := db.CreateNoteEx(svc.db, pid, "alpha", "", "", "other", "manual"); err != nil {
		t.Fatal(err)
	}
	configureStubEmbeddings(t, svc)

	base := []domain.SearchHit{{ID: 1, ProjectID: pid, Title: "alpha"}}
	svc.vecStore.store = downStore{}

	got := svc.fuseSemantic(base, "anything")
	if len(got) != len(base) {
		t.Fatalf("fuseSemantic returned %d hits, want the lexical input passed through unchanged (%d)", len(got), len(base))
	}
	if svc.vecStore.store != nil {
		t.Error("a failing store kept its memo slot: the next query would reuse a dead handle")
	}
	if name := svc.vectorStore().Name(); name == "down" {
		t.Errorf("re-resolution still yields the dead store (name=%q)", name)
	}
}

// Write path: RebuildEmbeddings holds one handle for the whole run, so a dead
// store has to be dropped on failure too — otherwise the user's retry after
// fixing their Qdrant/Weaviate config keeps the stale handle.
func TestRebuildEmbeddingsDropsDeadStore(t *testing.T) {
	svc, _ := setupService(t)
	configureStubEmbeddings(t, svc)
	pid := seedProject(t, svc.db, "rebuildproj", "/tmp/rebuildproj")
	if _, err := db.CreateNoteEx(svc.db, pid, "alpha", "", "", "other", "manual"); err != nil {
		t.Fatal(err)
	}
	svc.vecStore.store = downStore{}

	n, err := svc.RebuildEmbeddings()
	if err == nil {
		t.Fatalf("RebuildEmbeddings with a dead store: got nil error and %d embedded, want the failure surfaced", n)
	}
	if !errors.Is(err, errStoreDown) {
		t.Errorf("RebuildEmbeddings error = %v, want it to wrap the store error", err)
	}
	if svc.vecStore.store != nil {
		t.Error("a failing store kept its memo slot after a failed rebuild")
	}
}
