package service

import (
	"fmt"
	"strconv"

	"repo-nest/internal/db"
)

// ConfigData holds the application configuration sent to the frontend.
type ConfigData struct {
	Config    map[string]string `json:"config"`
	ScanRoots []string          `json:"scan_roots"`
}

// allowedConfigKeys is the allow-list of user-settable configuration keys.
var allowedConfigKeys = map[string]bool{
	"daily_code_standard": true,
	"scan_depth":          true,
	"git_author":          true,
	"auto_import":         true,
	// M1 Claude session auto-capture: "1" enables on-demand capture, anything
	// else (incl. unset) keeps it OFF. Reading session transcripts is
	// privacy-sensitive, so it is never implicitly enabled (ADR-0010).
	"claude_session_capture": true,
	// Target project for the agent-GLOBAL memory importers (openclaw / hermes).
	// Value is a project name or numeric id; unset -> those sources skip. See
	// memsrc.TargetProject and ADR-0011.
	"openclaw_project": true,
	"hermes_project":   true,
	// M3-A semantic search (ADR-0012). All default-OFF: semantic search runs
	// only when semantic_search=="1" AND a base_url+model are configured.
	// embedding_api_key is a SECRET and is redacted in GetConfig.
	"semantic_search":    true,
	"embedding_base_url": true,
	"embedding_model":    true,
	"embedding_api_key":  true,
	"embedding_dim":      true,
	// M3-A axis B (ADR-0013): vector STORE backend. Default local sqlite-vec;
	// vector_store="qdrant" + url (+api-key) opts into a remote/self-hosted
	// Qdrant, and any failure to reach it silently falls back to local.
	// vector_store_api_key is a SECRET and is redacted in GetConfig.
	"vector_store":            true,
	"vector_store_url":        true,
	"vector_store_api_key":    true,
	"vector_store_collection": true,
	// AI Q&A chat (OpenAI-compatible /chat/completions): LM Studio
	// (http://localhost:1234/v1) or any remote provider. ai_chat_api_key is
	// a SECRET and is redacted in GetConfig.
	"ai_chat_base_url": true,
	"ai_chat_model":    true,
	"ai_chat_api_key":  true,
}

// stringConfigKeys are exempt from the numeric-value check: they carry free-text
// (author name, project name/id, embedding endpoint/model/key, vector store).
var stringConfigKeys = map[string]bool{
	"git_author":              true,
	"openclaw_project":        true,
	"hermes_project":          true,
	"embedding_base_url":      true,
	"embedding_model":         true,
	"embedding_api_key":       true,
	"vector_store":            true,
	"vector_store_url":        true,
	"vector_store_api_key":    true,
	"vector_store_collection": true,
	"ai_chat_base_url":        true,
	"ai_chat_model":           true,
	"ai_chat_api_key":         true,
}

// secretConfigKeys are never returned in plaintext by GetConfig — a set value
// is masked so the frontend learns "configured" without seeing the credential.
// The backend still reads the real value via db.GetConfig directly.
var secretConfigKeys = map[string]bool{
	"embedding_api_key":    true,
	"vector_store_api_key": true,
	"ai_chat_api_key":      true,
}

// secretMask replaces a configured secret in responses sent to the frontend.
const secretMask = "********"

// GetConfig returns all configuration settings and scan roots.
func (s *Service) GetConfig() (*ConfigData, error) {
	configs, err := db.GetAllConfigs(s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	roots, err := db.GetScanRoots(s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to load scan roots: %w", err)
	}
	// A nil slice serialises to JSON null, which the frontend spreads/filters
	// directly and would throw. Always emit an empty array instead.
	if roots == nil {
		roots = []string{}
	}
	// Never return secret values in plaintext: a configured secret is masked to
	// a sentinel so the UI can show "set" without exposing the credential. The
	// backend reads the real value via db.GetConfig directly (e.g. the M3-A
	// embedder), never through this response.
	for key := range secretConfigKeys {
		if configs[key] != "" {
			configs[key] = secretMask
		}
	}
	return &ConfigData{Config: configs, ScanRoots: roots}, nil
}

// UpdateConfig sets a single configuration key-value pair after validating
// the key against the allow-list and numeric values.
func (s *Service) UpdateConfig(key, value string) error {
	if !allowedConfigKeys[key] {
		return fmt.Errorf("unknown config key: %s", key)
	}
	if !stringConfigKeys[key] {
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("config value must be a number")
		}
	}
	if err := db.SetConfig(s.db, key, value); err != nil {
		return err
	}
	// git_author is applied immediately so "mine" stats, heatmap and recent
	// commits reflect the new author without a restart.
	if key == "git_author" {
		s.setGitUser(value)
	}
	return nil
}

// UpdateScanRoots replaces the entire scan root list atomically.
//
// The submitted list is normalised first (see normalizeScanRoots): only real
// directories are stored, in canonical form, deduplicated. Entries that could
// not be accepted come back in Rejected instead of failing the whole call — the
// caller keeps the roots it asked for and learns exactly which ones were
// dropped and why.
//
// Storing the normalised list (rather than echoing the input back) is what
// keeps the persisted config and the scan itself in agreement: a root the
// scanner never visits must never appear to be configured.
func (s *Service) UpdateScanRoots(scanRoots []string) (*ScanRootsResult, error) {
	roots, rejected := normalizeScanRoots(scanRoots)
	if err := db.ReplaceScanRoots(s.db, roots); err != nil {
		return nil, fmt.Errorf("failed to update scan roots: %w", err)
	}
	// Both slices are always emitted as arrays, never null: the settings page
	// iterates them directly, and a null here throws a TypeError in the very tab
	// the user is looking at when the update succeeds.
	if roots == nil {
		roots = []string{}
	}
	if rejected == nil {
		rejected = []ScanRootRejection{}
	}
	return &ScanRootsResult{ScanRoots: roots, Rejected: rejected}, nil
}
