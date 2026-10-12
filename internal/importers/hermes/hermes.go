// Package hermes implements the built-in Hermes (Nous Research) agent-memory
// KnowledgeImporter (ADR-0011). Hermes is a product SEPARATE from OpenClaw. Its
// curated long-term memory lives as Markdown under the Hermes home dir:
//
//	~/.hermes/memories/{MEMORY.md,USER.md}      (root overridden by $HERMES_HOME)
//
// SECURITY: the Hermes home also contains `.env` (secrets), `mcp-tokens/`, and
// `state.db`. This importer is hard-scoped to memories/*.md ONLY — a flat,
// non-recursive listing of that one subdirectory — and never reads the parent,
// subdirectories, or any non-.md file.
//
// Sessions (`~/.hermes/sessions/`, `state.db`) are intentionally NOT imported:
// their record schema is not documented and cannot be verified on this machine,
// so a lenient-but-correct parser is deferred until the format is confirmed
// (ADR-0011 决策 3 / 5 — never guess a live format).
//
// Hermes memory is agent-GLOBAL, so notes attach to the project named by the
// `hermes_project` config value; unset/unknown -> skipped. Registered as an
// OPT-IN manual source, excluded from startup auto-import (ADR-0011 决策 4).
package hermes

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"repotory/internal/core/plugin"
	"repotory/internal/db"
	"repotory/internal/importers/memsrc"
)

// SourceName is the stable knowledge-source identifier for Hermes memory.
const SourceName = "hermes"

// ProjectConfigKey names the config value (project name or numeric id) that
// Hermes's global memory notes attach to.
const ProjectConfigKey = "hermes_project"

const maxReadBytes = db.MaxNoteContentLen + 1

// Importer implements plugin.KnowledgeImporter for Hermes curated memory.
type Importer struct {
	db *sql.DB
}

// New creates a Hermes importer bound to the application database.
func New(database *sql.DB) *Importer {
	return &Importer{db: database}
}

// Source returns the stable source identifier "hermes".
func (i *Importer) Source() string { return SourceName }

// memoriesDir resolves <hermesHome>/memories, honoring $HERMES_HOME over ~/.hermes.
func memoriesDir() (string, bool) {
	if h := strings.TrimSpace(os.Getenv("HERMES_HOME")); h != "" {
		return filepath.Join(h, "memories"), true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(home, ".hermes", "memories"), true
}

// Import reads ONLY <hermesHome>/memories/*.md (flat) and yields one note per
// non-empty file. Returns nil (no-op) when Hermes is not installed.
func (i *Importer) Import() ([]plugin.ImportDoc, error) {
	dir, ok := memoriesDir()
	if !ok {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil // no Hermes memories dir: successful no-op
	}
	pid := memsrc.TargetProject(i.db, ProjectConfigKey)

	var docs []plugin.ImportDoc
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue // allowlist: top-level markdown only, no recursion
		}
		raw, err := memsrc.ReadCapped(filepath.Join(dir, e.Name()), maxReadBytes)
		if err != nil {
			continue
		}
		body := memsrc.StripFrontmatter(string(raw))
		if strings.TrimSpace(body) == "" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		content := "> 由 Repotory 从 Hermes 全局记忆导入（source: hermes，allowlist: memories/*.md）\n\n" + body
		content = memsrc.ClipToBytes(content, db.MaxNoteContentLen)
		docs = append(docs, plugin.ImportDoc{
			ProjectID: pid,
			Title:     "Hermes · " + name,
			Content:   content,
			Kind:      "knowledge",
			Tags:      "hermes",
			Source:    SourceName,
		})
	}
	return docs, nil
}
