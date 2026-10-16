---
title: Dashboard
order: 2
---

# Dashboard

The dashboard shows daily commit statistics, goal progress, and a year-long commit heatmap across all projects.

> **Merged into Home (ADR-0024)**: the dashboard panels are now the body of the single-column **home workbench** — the app opens straight onto this content; the main navigation swaps "Dashboard" for "Search", and old `/dashboard` links redirect to Home. This page keeps the "Dashboard" title for the project-status panels on the home screen (one top-to-bottom column, no longer side-by-side with the memory ring).

## Page structure (top to bottom)

```mermaid
flowchart TB
    STRIP["Memory-ring strip<br/>recent handoffs · global search entry"] --> RING["① Goal progress<br/>progress ring · summary bar"]
    RING --> HEAT["② Commit heatmap<br/>last 52 weeks · click to jump to a day"]
    HEAT --> CTRL["③ Controls<br/>date switch · combined search<br/>filters · sorting · rescan"]
    CTRL --> GRID["④ Project grid"]
    GRID --> STAR["Starred: full stat card<br/>goal bar · team total"]
    GRID --> PLAIN["Unstarred: minimal card<br/>name + star button"]
```

How to read it: the page is a **vertical reading order** with a memory-ring strip on top, then four bands. The heatmap (②) is the global time view, the control bar (③) is its operating panel (switching the date changes both the heatmap and the project cards at once), and the project grid (④) splits cards by starred state — unstarred shows only the name, and you have to star a repository before it shows statistics. All three bands follow the **git author setting** (see [Settings](settings.md#author-configuration)).

### Memory-ring strip (page top, ADR-0024)

A horizontal memory band at the very top of the home screen: the "Memory Ring" label on the left, **recent session handoffs** in the middle (click to open the owning project; auto-captured ones carry an "Auto" badge), and the **global search entry** on the right — it opens the standalone [global search page](search.md). With no handoffs yet, a placeholder hint is shown; on narrow screens the band wraps.

### Goal progress

- **Progress ring**: today's (or the selected date's) personal added lines vs the daily goal; no goal is shown on non-workdays
- **Subtitle**: how far you are from the goal, personal additions / file count / repos involved
- **Summary bar**: team additions / deletions, personal additions / files, total todo count

### Commit heatmap

Daily commit density over the last 52 weeks (GitHub style). Click any cell to jump to that date and see that day's per-project data. Stats follow the current git author (Settings → Author configuration).

### Controls

- **Date switcher**: yesterday / today / any date
- **Unified search box**: search repos, notes, and todos as you type (300ms debounce; results can be starred or opened directly)
- **Filter**: all / starred only
- **Sort**: name / personal additions / file count / repo count
- **Rescan**: starts an async scan after a two-click confirmation; progress is shown live and data refreshes automatically when the scan completes

### Project grid

- **Starred repos**: full stat cards (today's additions, goal progress bar, repos / files / additions / deletions, net growth, team totals, todo and note badges)
- **Other repos**: minimal cards (name + star button); star one to expand it when needed

## Feature details

### Starring repos

Click the star to star a repo: the card expands into full stats and a **backfill history** button appears (backfills the last 365 days of data on demand). Unstarring collapses it back to a minimal card immediately.

### Goal progress

**Settings → Code standard** sets the daily goal in lines (default 500, range 100-10000).

- The progress ring and card goal bars show completion; hitting the goal shows 🎉
- On workdays below the goal, cards show a "below target" warning; weekends never warn

### Project grouping

Projects are grouped automatically by parent directory (with Monorepo detection). To split/merge manually, see [Project detail](project-detail.md).
