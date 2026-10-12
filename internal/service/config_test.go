package service

import (
	"encoding/json"
	"strings"
	"testing"

	"repotory/internal/db"
)

// Regression: a nil scan-root slice serialises to JSON null, and the settings
// page spreads/filters the field directly ([...data.scan_roots]), so adding the
// very first scan root threw a TypeError. Every slice/map in the config payload
// must serialise as an empty collection, never null.
func TestGetConfig_NoNullCollections(t *testing.T) {
	svc, _ := setupService(t)

	cfg, err := svc.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg.ScanRoots == nil {
		t.Error("ScanRoots is nil -> serialises to null")
	}
	if cfg.Config == nil {
		t.Error("Config is nil -> serialises to null")
	}

	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("config JSON contains null: %s", b)
	}
}

// TestConfigSecretMasking covers M3-A config keys: settable, numeric-validated,
// and the embedding_api_key secret never leaves GetConfig in plaintext.
func TestConfigSecretMasking(t *testing.T) {
	svc, _ := setupService(t)

	set := func(k, v string) error { return svc.UpdateConfig(k, v) }
	if err := set("semantic_search", "1"); err != nil {
		t.Fatalf("semantic_search: %v", err)
	}
	if err := set("embedding_model", "text-embedding-3-small"); err != nil {
		t.Fatalf("embedding_model (string): %v", err)
	}
	if err := set("embedding_base_url", "http://localhost:11434/v1"); err != nil {
		t.Fatalf("embedding_base_url: %v", err)
	}
	if err := set("embedding_api_key", "sk-super-secret"); err != nil {
		t.Fatalf("embedding_api_key: %v", err)
	}
	if err := set("embedding_dim", "1536"); err != nil {
		t.Fatalf("embedding_dim numeric: %v", err)
	}
	// Non-numeric on a numeric key must be rejected.
	if err := set("embedding_dim", "abc"); err == nil {
		t.Error("embedding_dim should reject a non-numeric value")
	}

	cfg, err := svc.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Config["embedding_api_key"] != secretMask {
		t.Errorf("api key not masked in GetConfig: %q", cfg.Config["embedding_api_key"])
	}
	if cfg.Config["embedding_api_key"] == "sk-super-secret" {
		t.Error("LEAK: raw api key returned to frontend")
	}
	// Non-secret keys are returned verbatim.
	if cfg.Config["embedding_model"] != "text-embedding-3-small" {
		t.Errorf("embedding_model = %q", cfg.Config["embedding_model"])
	}
	if cfg.Config["semantic_search"] != "1" {
		t.Errorf("semantic_search = %q", cfg.Config["semantic_search"])
	}
	// The backend can still read the REAL secret (what the embedder needs).
	if raw, err := db.GetConfig(svc.db, "embedding_api_key"); err != nil || raw != "sk-super-secret" {
		t.Errorf("raw secret read via db = %q err=%v, want sk-super-secret", raw, err)
	}
}
