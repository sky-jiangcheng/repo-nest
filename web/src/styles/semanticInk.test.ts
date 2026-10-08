import { describe, it, expect } from 'vitest'

// The semantic ink classes (.green / .red / .muted-num) are bare utilities at
// specificity (0,1,0). Every number class that renders one ALSO sets `color` at
// (0,1,0) — so whichever rule appears later in the stylesheet wins, and a
// semantic colour can be silently flattened to neutral ink.
//
// This already happened twice here: `.card-stat .stat-value` beat a bare
// `.green`, and later `.flat-num` (declared ~540 lines after `.green`) beat it
// again — every +/- number on the flat project card rendered grey. Nothing
// caught it: the build passes, tsc passes, and CI is blind to CSS. So the
// invariant is pinned here instead.
//
// The files are read through node:fs rather than import.meta.glob('?raw'),
// which returns EMPTY strings for .css under vitest (verified — a glob-based
// version of this test passed against nothing). The specifier is assembled at
// runtime so tsc does not require @types/node, which this project does not
// depend on; `npm run build` type-checks src/ and would otherwise fail on a
// test-only import.

// Number classes that declare their own `color`, paired with the semantic
// classes they render alongside. Keep in sync with the assertions below —
// adding a new number class means adding it here too.
//
// .flat-num is NOT in this list anymore: ProjectCard was migrated to
// ProjectCard.module.css (Sprint 14, TODO P35), where the number class (.num)
// and its ink (.numSuccess/.numDanger/.numMuted) are module-scoped — a CSS
// module hash-scopes both, so the global-cascade ordering hazard this file
// guards against cannot occur between them. If a component ever renders
// global .flat-num again, re-add it here AND restore the base rule in
// dashboard.css.
const pairs: Array<[string, string[]]> = [
  ['summary-value', ['green', 'red', 'zero']],
  ['head-stat-value', ['green', 'red']],
]

// Every stylesheet in the cascade, in the order styles/index.css imports them.
// Order matters for specificity ties, which is the whole point of this file.
const files = [
  'tokens.css',
  'reset.css',
  'typography.css',
  'components/buttons.css',
  'components/inputs.css',
  'components/cards.css',
  'components/toast.css',
  'components/messages.css',
  'components/command-palette.css',
  'components/tabs.css',
  'components/search.css',
  'layouts/navbar.css',
  'layouts/main.css',
  'features/dashboard.css',
  'features/project-detail.css',
  'features/knowledge.css',
  'features/notes.css',
  'features/markdown.css',
  'features/heatmap.css',
  'features/todos.css',
  'features/settings.css',
  'features/block-editor.css',
  'features/status-bar.css',
  'features/fab.css',
]

const fsName = 'node:' + 'fs'
const pathName = 'node:' + 'path'
const procName = 'node:' + 'process'

interface NodeFs {
  readFileSync(p: string, enc: string): string
}
interface NodePath {
  resolve(...p: string[]): string
  join(...p: string[]): string
}
interface NodeProcess {
  cwd(): string
}

async function readAllCss(): Promise<string> {
  // Typed structurally rather than via `typeof import('node:fs')`: naming the
  // module in a type position makes tsc demand @types/node, which this project
  // does not depend on.
  const fs = (await import(/* @vite-ignore */ fsName)) as unknown as NodeFs
  const path = (await import(/* @vite-ignore */ pathName)) as unknown as NodePath
  const proc = (await import(/* @vite-ignore */ procName)) as unknown as NodeProcess
  const dir = path.resolve(proc.cwd(), 'src/styles/design-system')
  return files.map(f => fs.readFileSync(path.join(dir, f), 'utf8')).join('\n')
}

describe('semantic ink pairing', () => {
  it('every number class paired with a semantic class has a doubled-class rule', async () => {
    const allCss = await readAllCss()
    const missing: string[] = []
    for (const [base, semantics] of pairs) {
      for (const sem of semantics) {
        // Match `.base.sem` as a whole token, so a bare `.green` (0,1,0)
        // cannot satisfy this by accident.
        const re = new RegExp(`\\.${base}\\.${sem}(?![\\w-])`)
        if (!re.test(allCss)) missing.push(`.${base}.${sem}`)
      }
    }
    expect(missing).toEqual([])
  })

  it('each number class still sets color, so a deleted pairing would really regress', async () => {
    // Documents that the base rules are load-bearing: they are what the doubled
    // rule has to outrank. If one stopped setting `color`, the pairing would
    // become a silent no-op instead of a fix, and this list would be stale.
    const allCss = await readAllCss()
    for (const [base] of pairs) {
      const re = new RegExp(`\\.${base}\\s*\\{[^}]*color:`, 'm')
      expect(re.test(allCss), `.${base} no longer sets color — revisit the pairs list`).toBe(true)
    }
  })

  it('the bare utilities still exist and carry the token values', async () => {
    const allCss = await readAllCss()
    expect(allCss).toMatch(/^\.green\s*\{\s*color:\s*var\(--success\)/m)
    expect(allCss).toMatch(/^\.red\s*\{\s*color:\s*var\(--danger\)/m)
  })

  it('a number class declared after the bare utility has a doubled rule to survive it', async () => {
    // The regression that greyed out the project card was purely an ordering
    // accident. This test names that dependency: a number class written after
    // `.green` at the same specificity is only safe because the doubled rule
    // exists, so whoever moves these blocks next finds the reason here.
    const allCss = await readAllCss()
    const greenAt = allCss.search(/^\.green\s*\{/m)
    expect(greenAt).toBeGreaterThan(-1)
    for (const [base] of pairs) {
      const at = allCss.search(new RegExp(`^\\.${base}\\s*\\{`, 'm'))
      if (at <= greenAt) continue
      const hasDoubled = new RegExp(`\\.${base}\\.(green|red)`).test(allCss)
      expect(
        hasDoubled,
        `.${base} is declared after .green at equal specificity; without a doubled rule its numbers render grey`,
      ).toBe(true)
    }
  })
})
