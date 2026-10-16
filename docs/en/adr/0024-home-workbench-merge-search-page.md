# ADR-0024: Home page workbench — merge home with dashboard, split global search into its own page, add recommendations

- Status: **Accepted (2026-10-16, proposed directly by the user)**
- Date: 2026-10-16
- Related: [ADR-0017](0017-one-kernel-two-entry-points.md) (this ADR revises the two-column side-by-side wording in its UI-alignment section; the principle "dashboard stays on the first screen, is not hidden by default" remains unchanged), [ADR-0006](0006-scope-freeze.md) (the dashboard remains a supporting capability; this ADR adds no new backend features)

## Background

After D25 / ADR-0017 landed, `/` (Home) rendered as "left column: memory ring + right column: full embedded `<Dashboard />`" in a two-column side-by-side layout, while the `/dashboard` route independently rendered the same Dashboard component again. This produced three problems:

1. **Duplicate rendering**: the same "goal ring + heatmap + summary + project grid" rendered in two top-level routes; two navigation entries pointed to the same content; every future dashboard change had to account for two containers.
2. **Layout debt**: the two-column side-by-side layout stacked unevenly at the ≤1080 px breakpoint — ADR-0017 already fixed a real defect on the day of promotion (memory ring collapsed, search box and handoff list hidden under the dashboard); on narrow screens, the two-column layout was essentially a forced fit.
3. **Search had no landing spot**: global search was scattered across three places (an inline input in Home, the ⌘K CommandPalette, and the dashboard's project search dropdown) with no directly reachable top-level entry; the "empty state" of search — hot projects, hot tags — had nowhere to live inside the command palette; a cold-start user opening search would see only a blank input box.

## Decisions

1. **Home = workbench (single-column fusion)**: `/` redesigned as a top-to-bottom narrative flow: today's goal hero (goal ring + summary) → heatmap → memory ring bar (recent handoff items in a horizontal strip + a prominent "global search" entry point) → control row (date / filter / sort / rescan) → project grid → status bar. The inline search block and handoff column in Home's left column are removed. The `/dashboard` route becomes a redirect to `/`; the main navigation drops the "Dashboard" item and adds a "Search" item. The Dashboard component is retained as a panel referenced from the home page; no features removed, not hidden by default.
2. **Global search as its own top-level page `/search`**: a large search box with autofocus; debounced search over notes and todos; results grouped into "Projects / Notes / Todos"; the query syncs to the URL via `?q=` for deep-linking. The ⌘K CommandPalette remains as the keyboard quick-switcher; its first action (new handoff) is unchanged.
3. **Recommendations land in the `/search` empty state**: with no query, a recommendation area shows the top-6 hot projects (sorted by recent personal added-line count descending, ties broken by star count and repo count, all zeros fall back to default order) and hot tag chips (clicking a tag fills it into the search box and executes the search). Sorting is purely front-end; no backend fields added, no schema changes.

## Rationale

- Duplicate rendering is real maintenance cost and positioning noise; the single-column fusion turns "memory ring + project status" from "two columns squeezing each other" into a single narrative stream, preserving ADR-0017's intent (both visible on the same screen, narrative aligned) while removing the narrow-screen layout exception that had been a long-standing problem.
- Search deserves a page that can carry "empty-state value": with no query, hot projects and tags give a foothold that a transient command palette cannot; a top-level page also gives stable anchors for the manual, keyboard shortcuts, and external links.
- Not touching the backend was deliberate: existing endpoints already supply all fields needed for sorting (`my_added` / `is_starred` / `repo_count` / `ListAllTags`); this is a pure information-architecture change, leaving schema and statistics upgrades for a future with real demand signals.

## Consequences

- **Positive**: duplicate routing eliminated; narrow-screen layout complexity reduced; search gains a top-level entry and a recommendations slot; `Home.test` / `Search.test` directly pin the new structure.
- **Negative**: old `/dashboard` links (bookmarks, doc references) will be redirected — a one-time migration; the two-column wording in ADR-0017 is revised by this ADR, and readers of the old text should reference the note; hot ranking is a client-side heuristic (no historical activity weighting); tags have no popularity counter, and "hot" is defined by the page's labels.
- **Boundary**: dashboard assets (goal ring, heatmap, working-day alerts) remain on the first screen; ADR-0017's stance of "do not remove from first screen, do not hide by default, do not delete" is unchanged — the only change is the container: from an independent route merged into the home page, and two columns side-by-side replaced by a single-column fusion.
