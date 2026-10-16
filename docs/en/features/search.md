---
title: Global Search
order: 3
---

# Global Search

Global search is a top-level page (ADR-0024): one input that searches **notes, todos, and project names across all projects**, with results grouped into Projects / Notes / Todos. Both the "Search" item in the navigation bar and the entry on the right of the home memory-ring strip lead here; the ⌘K / Ctrl+K command palette remains the keyboard-first quick switcher (its first action is still "new handoff") — the two coexist.

## Page structure

- **Search box**: focused on entry; searching debounces at 150ms, and the query is mirrored into the address bar as `?q=` (bookmarkable and deep-linkable from other pages)
- **Result groups**: projects (name match) / notes / todos (FTS5 full-text search with keyword highlights), each group with a count
- **No results**: a hint is shown, suggesting to start from a hot tag

## Idle recommendations

With an empty input, the page shows two recommendation groups (instead of a blank box):

- **Hot projects** (top 6): ranked by **recent personal added lines** (`GetProjects`'s default yesterday anchor), with starred state and repo count as tiebreakers; click to open the project
- **Hot tags** (top 12): tags from all notes; **clicking one fills the input and runs the search**

"Hot" is a client-side heuristic (ADR-0024); the ranking basis is labelled on the page itself.

## How it relates to the other search entries

| Entry | Scope | Notes |
|------|------|------|
| Global search page | notes · todos · project names | Top-level page, deep-linkable, idle recommendations |
| ⌘K command palette | notes · todos · project names | Keyboard quick switching, first action "new handoff" (see [Command Palette](command-palette.md)) |
| Home unified search box | repos, notes, and todos | Results can star a repo directly (see [Command Palette](command-palette.md)) |
