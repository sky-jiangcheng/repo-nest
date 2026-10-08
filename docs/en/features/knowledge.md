---
title: Knowledge Base and Notes
order: 3
---

# Knowledge Base and Notes

The knowledge base is the app's home page and the hub for cross-project notes: Markdown / block editor, tag categories, FTS5 full-text search, version history, and AI memory import.

```mermaid
flowchart TB
    NEW["Create a note<br/>home / project detail"] --> ED["Block editor<br/>Markdown ↔ blocks"]
    ED --> AUTO["Autosaved draft"]
    AUTO --> SNAP["Save → snapshot<br/>latest 50 kept"]
    SNAP --> DIFF["LCS<br/>line-level diff"]
    DIFF --> BACK["One-click<br/>restore"]
    ED --> IDX[("FTS5 index<br/>trigram · trigger-synced")]
    IDX --> SEARCH["Search / ask<br/>snippet highlights"]
        classDef store fill:#fffbeb,stroke:#f59e0b,color:#78350f
        classDef read fill:#f0fdf4,stroke:#22c55e,color:#14532d
    class IDX store
    class SEARCH read
```

How to read it: one **trunk (edit → save → snapshot → diff → restore)** plus one **branch (index → search)**. The trunk solves "knowledge must not be lost"; the branch solves "knowledge must be findable" — both are triggered by the same save action (snapshots via a write trigger, the index via the FTS sync trigger), so there is no save button to remember. The **History** button on a note card enters the version side; the search box enters the branch.

## Managing notes

- **Create a note**: pick a project in "Quick create note" on the home page, or create one from the notes panel on the project detail page
- **Metadata**: title (falls back to the first line if left empty), tags (comma-separated), category (knowledge / log / idea / other), pinned
- **Move across projects**: use the "Linked project" dropdown while editing to migrate in one click
- **Autosaved drafts**: edits are saved locally in real time, so nothing is lost if the app closes unexpectedly

## Block editor

Type `/` to open the block panel and insert structured blocks:

| Block | Description |
|----|------|
| Callout | TIP / WARNING / NOTE callouts |
| Code block | with language highlighting |
| Mermaid diagram | flowcharts / sequence diagrams, etc. |
| Math formula | rendered with KaTeX |
| Todo list / table / divider | common structures |
| Collapsible block / Tabs | `<details>` and `{% tabs %}` |

- Blocks can be drag-sorted, moved up or down, and deleted individually
- **Switch between Markdown and blocks at any time**: the storage format is always plain Markdown, so any editor can open it

## Rich rendering

highlight.js code highlighting, Mermaid diagrams, KaTeX math formulas, GFM callouts, and task lists.

## Search

- The home page search box is auto-focused; "Ask the knowledge base" mode returns answer snippets ranked by relevance
- FTS5 trigram + bm25 ranking, with snippets highlighting matched terms; short CJK queries automatically fall back to LIKE
- Covers notes and todos; the `⌘/Ctrl+K` command palette works from anywhere

### Semantic search (optional, off by default)

Lexical search answers "which characters matched", not "the same fact phrased differently". Optional vector recall covers exactly that blind spot:

- **Two-stage fusion**: FTS5 and vector recall are merged with RRF (k=60); vectors only ever **add** — if any step fails (endpoint unset, request error, index missing) it falls back to the pure lexical result, so **turning it on cannot return fewer hits**. That is a design constraint, not a reassurance.
- **Off by default**: active only when `semantic_search=1` **and** an embedding endpoint is configured (`embedding_base_url` + `embedding_model`). The settings page does not expose these yet; enable them via the configuration keys described in [AI Integration](ai-integration.md).
- **The first time needs one full rebuild**: every note's text is embedded, which is an O(all notes) walk of the endpoint. It also persists the vector dimension it discovers.
- **No further rebuilds afterwards**: note creation, title/body edits and deletions are queued by SQLite triggers and drained in the background every 5 seconds — live notes get re-embedded, deleted notes are removed from the vector index (otherwise text you deleted keeps showing up in recall). Three deliberate conservations: pinning, retagging and moving a note do **not** trigger an embed (they do not change the text sent to the endpoint); when the endpoint is unavailable the work stays queued and retries, so an offline laptop loses nothing; and when the dimension is unknown or the index does not exist yet it skips rather than guessing.
- **Imported notes are indexed the same way**: queueing happens in database triggers, and every writer (UI, MCP, CLI, agent-memory importers) converges on that layer — which is precisely why this was not built into application logic.
- **Privacy boundary**: enabling semantic search means note text is sent to the embedding endpoint you configured (local Ollama or a remote API, depending on `embedding_base_url`). That is an explicit choice, off by default; `embedding_api_key` and `vector_store_api_key` are secrets, returned masked when configuration is read and never handed to the UI.
- **The vector index is a derived cache**; the notes themselves are the source of truth: it can be dropped and recomputed at any time, living in the same database file as the notes, in the same transactions and the same backups (pure-Go sqlite-vec, zero CGO). A remote vector store is only worth reaching for at real scale — see [AI Integration](ai-integration.md).
- **Whether it helps is measurable before it ships**: `cmd/abeval` runs "lexical vs lexical+vector" against a live database and reports Recall@k / NDCG@k with a threshold gate. The gate comes before the switch deliberately — until there is a real labelled query set, no UI toggle ships.

## Version history

A snapshot is created automatically on every save (the latest 50 are kept). The **History** button on the note card opens:

- A version list (time, title)
- A line-level LCS diff of any version vs the current one (+/- markers)
- One-click restore to any historical version

## Import knowledge from agent memory

Six built-in sources idempotently import the memory and session transcripts you already left in other agent tools as knowledge notes (re-importing updates existing notes instead of duplicating them):

| Source | Trigger | Reads |
|--------|---------|-------|
| `claude` | automatic at startup | `~/.claude/projects/*/memory/*.md` |
| `codex` | manual | session transcripts `rollout-*.jsonl` (first instruction + last reply) |
| `opencode` | manual | session titles and summaries |
| `openclaw` | manual | `~/.openclaw-autoclaw/workspace/*.md` |
| `hermes` | manual | `~/.hermes/memories/{MEMORY,USER}.md` |
| `cursor` | manual | Cursor sessions inside `globalStorage/state.vscdb` (best effort, see below) |

**Settings → Plugins** lists every source, lets you trigger each one manually, and shows the per-source `{created, updated, skipped}` counters.

Three rules that are easy to trip over:

1. **Imports land on a project, they do not pile onto the knowledge home page.** Documents are matched to a specific project by project name / repository path; whatever does not match is counted as `skipped` and not stored. So scan the projects first — hit rate depends on that order.
2. **`openclaw` and `hermes` need their target project configured first** (`openclaw_project` / `hermes_project`); otherwise the entire source is silently skipped.
3. **A repeat import is not a failure.** `created=0` plus `updated=N` means idempotency worked and the content was refreshed.

**`cursor` is the only best-effort source**: Cursor's on-disk format is undocumented and versioned, so it takes only the **first prompt and last reply** of each session (never a full transcript), resolves ownership through `workspaceStorage/<id>/workspace.json` and skips what it cannot match rather than guessing — and when the database shape is not one it recognizes, **the whole source fails loudly instead of quietly importing zero notes** (because "nothing imported" must stay distinguishable from "you have no sessions"). Every read opens that file `mode=ro`; another application's data is never written.

Imported notes carry `kind` = `knowledge` (memory files) or `log` (session records) and are findable by full-text search as soon as they are written; with semantic search enabled they are also picked up by vector recall without another rebuild (see above). Full paths, matching rules and limitations per source are in [Knowledge source plugins](../plugins/overview.md).

## Export

- **Export .md** on the note card copies the Markdown — with YAML frontmatter (title / tags / project / type / updated time) — to the clipboard. For bulk AI consumption, see llms.txt in [AI integration](ai-integration.md).
- **OMP-style memory export** (`ExportMemoryJSON`): dumps the knowledge base as OMP (Open Memory Protocol) style Memory Objects for other tools speaking that protocol. **Export only — there is no import side**, and the field mapping is still marked provisional: the protocol itself is not stable, so two-way sync was deliberately left undone. It is reachable through the app binding layer; there is no dedicated button yet.
