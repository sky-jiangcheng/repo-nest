# ADR-0017: One kernel, two entry points — product layering without forking the repo

- Status: Proposed
- Date: 2026-10-08
- Relates to: [ADR-0006](0006-scope-freeze.md) (scope freeze and feature tiering), [ADR-0008](0008-pwa-removal.md) (legacy cleanup), [ADR-0009](0009-ide-presence.md) (distribution strategy), TODO D25 (dashboard trim; this ADR revises its remove/hide tendency), docs/positioning-brief.md

## Context

The product narrative has converged on "local-first, cross-agent project memory layer" (the `scan → context → handoff` loop, see positioning-brief), but the repository still carries two narratives side by side: the **agent side** (13 MCP tools, CLI, init script, multi-agent importers) and the **human side** (desktop app: Dashboard, project state, goal ring, heatmap, knowledge editing). When asking "should we split into two products", two different questions must be separated:

1. **Splitting code/data**: `internal/service` (single repo, single SQLite schema, version SSOT in `wails.json`) is shared by the desktop app, the `mcp` binary, `reponest-server`, the CLI and IDE extensions. The cross-agent loop (Agent A writes a handoff → human reads/edits in the GUI → Agent B reads it) depends on that single data substrate. Physically splitting into two products means duplicating the whole backend, aligning two version trains, and cutting the loop into two islands — destroying the only real moat.
2. **Layering narrative/entry points/UI**: this is what "splitting" actually wants to fix — the first screen only shows the dashboard while the memory loop sits in a deep menu, so the public promise and the opened app disagree (the positioning-purity drain D25 points at). It can be done without a second repo or a forked schema.

Maintenance reality: ~438 commits, effectively all from a single person. Splitting into two standalone products would stack twice the releases, docs and test matrices onto the same shoulders.

## Decision

**One kernel, two entry points. No second repository, no forked schema, no duplicated service layer.**

### 1. One kernel (unchanged)

Single repository, single SQLite schema, single `internal/service`, single version number (`wails.json` SSOT). All "entry points" keep sharing this kernel. This ADR explicitly forbids creating a second repository or a second schema for product-layering purposes.

### 2. Two entry points (product layering, not forking)

- **Agent side (primary narrative)**: the headless toolchain — MCP server (standalone single-file binary) plus `reponest-capture` / `reponest-init` CLI. This is the "memory layer" itself, built for AI agents and scripts, with no GUI dependency.
- **Human side (view, edit and project-understanding surface)**: desktop app + IDE extension. It is both the read/write interface over the knowledge base and the human-side expression of the "project understanding" capability (README / tech stack / dependencies / contributors / activity). **The human side is half of the product, not an accessory** — Dashboard, goal ring, heatmap and workday alerts are good user-side features, positioned as the capability outlet for "project understanding", and this ADR explicitly keeps them.

### 3. Narrative split

README, docs site and release notes lead with the "agent memory layer" (three-tool loop + per-agent onboarding); the Desktop App section becomes "Human-side view / editing", while honestly keeping its project-understanding capability.

### 4. UI alignment (memory loop and project state, one screen)

The first screen presents "memory-loop entry + project-state overview" side by side: recent `handoff`s and global search are the memory-loop entry; Dashboard, goal ring, heatmap and workday alerts stay **on the same screen** as the human-side expression of "project understanding" — not moved off the first screen, not hidden by default, not demoted, not deleted. This revises D25's "move off first screen / hide by default / sink into a plugin" tendency: D25 correctly identified the mismatch ("the first screen only talks dashboard, the memory loop is invisible"), but its remove/hide direction is superseded by this ADR.

### 5. Release split (formalize what already exists)

The `mcp` binary, `reponest-server`, desktop installer and VS Code extension ship and download independently, but share one version number and one CHANGELOG; the headless path (`node scripts/reponest-init/index.mjs --with-hook`) is the first install path in both the docs site and README.

## Consequences

Positive: narrative purity is restored — "two entry points" is a factual description of the status quo instead of a fight with the public promise; maintenance stays single-track (one repo, one schema); the loop's data never splits; agent-first users never need to open the GUI; the human-side "project understanding" experience is fully preserved, untouched by the narrative change. Negative: the first screen carries more information and needs finer layout plus a visual-regression pass; with the memory loop and the dashboard on one screen, new users still need copy to understand the "cross-agent memory" claim rather than stopping at the dashboard narrative; the UI change itself needs a one-off regression run and a migration note.

## Open questions

1. **When would an actual split into separate repos be justified?** Suggested hard preconditions: ① a second, mutually exclusive user group emerges (pure-headless agent authors vs pure-human knowledge-base users) with long-term diverging needs; ② at least 1–2 community maintainers each commit to owning one product line; ③ the MCP side gets adopted by external frameworks as a standard memory interface. Re-evaluate only if any of these materialize; do not split while single-maintained.
2. **Dashboard assets**: **Decided — keep.** The dashboard (Dashboard, goal ring, heatmap, workday alerts) is the human-side expression of the core "project understanding" capability and is kept in full as a good user-side feature: not deleted, not hidden by default, not sunk into a plugin. D25's "move off first screen / hide by default / sink into a plugin or delete" tendency is superseded by this ADR; when executing D25 later, only put the memory-loop entry on the first screen — no feature subtraction. **External review note (2026-10-08)**: an external review again proposed "cutting or demoting non-loop features such as the dashboard"; after evaluation this conflicts with this item and the opinion is internally inconsistent (its value judgment admits the human-side experience must be kept), so it is not adopted — this item stands as decided.
3. **Possible future artifacts**: should handoff archive export (memory export) and knowledge-base export (knowledge export) be separate downloads? Registered here, not decided.
4. **Target user persona (inspired by an external review)**: the external review suggests early adopters are "people using multiple agents, switching across several local repos, and valuing long-term context and handoffs", judged to be "a smaller but sharper-need" group. This ADR adopts that profile as a candidate outward persona, but "smaller but sharper" is currently **unvalidated** — should real user research (interviews/surveys) be run to verify existence and need intensity before converging the outward narrative? Registered here, not decided.
5. **Acceptance criteria for auto-capture (ties to ADR-0010)**: the external review stresses "auto-capture session wrap-up by default; do not rely on a manual handoff every time". ADR-0010's auto-capture has stayed at "accepted in principle" without promotion — should a "zero-action trigger ratio" (share of sessions reliably captured with no manual action) be added as an acceptance criterion to drive its promotion? Registered here, not decided.
6. **Evidence for "context is clearly better than manual retrieval"**: the outward claim "one call is clearly better than the agent reading README, TODO and git log itself" is currently an assertion, not a measurement — should a comparative evaluation be built (annotated retrieval-quality comparison of `reponest_context` vs manually assembled context on the same query) with a passing bar folded into acceptance? Registered here, not decided.

## Promotion criteria (Proposed → Accepted)

1. README first screen and docs-site narrative lead with the agent memory layer; desktop wording is uniformly "human-side view", with the "project understanding" capability described completely and without stale claims.
2. GUI first screen shows both the memory-loop entry (recent handoffs + global search) and the project-state overview (including Dashboard etc.); functional tests plus visual regression pass for both the memory loop and the dashboard.
3. The headless path (MCP binary + init script) is the first install path in README and docs; desktop and headless share one version number and one CHANGELOG.
4. Throughout: no new repository, no forked schema, no duplicated service layer (verifiable at the git level).

## Appendix: release-strategy checklist (in execution order)

1. **README rework**: first screen shows the three-tool loop + agent onboarding list (Claude Code / Cursor / OpenCode / Codex…); the "Desktop App" section becomes a human-side view subsection that keeps the project-understanding capability description; run a stale-claim sweep across README / SKILL.md / docs (same approach as D24).
2. **Docs-site changes**: getting-started is reordered headless-first (`reponest-init --with-hook` first); add an "Agent-side vs Human-side" comparison; `features/dashboard` is kept as the "project understanding" human-interface doc and no longer marked legacy.
3. **Release artifacts**: the Release page lists the `mcp` binary and the desktop installer side by side, each downloadable; CHANGELOG stays unified.
4. **GUI alignment (revising D25's tendency)**: the default route becomes a "memory loop + project state" combined first screen; Command Palette's first action becomes "new handoff"; Dashboard and related component tests stay and remain visible by default — no hiding.
5. **Acceptance**: existing tests (Go + frontend) plus CSS/visual regression all green; docs consistency check (README/SKILL/docs say the same thing); git-level confirmation that repo, schema and version SSOT are unchanged.
6. **Trigger registration**: the three split preconditions from Open question 1 are recorded in this ADR as the only basis for any future "should we really split" decision.