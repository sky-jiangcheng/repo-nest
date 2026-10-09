// NotFound.test.tsx — regression test for the catch-all route.
//
// The bug this guards: <Routes> had no path="*" entry, so any URL outside the
// registered set rendered nothing at all — a blank page below the navbar that
// was indistinguishable from a crash. The dev server returns HTTP 200 for
// unknown paths (SPA fallback), so nothing surfaced the mistake; it only showed
// up when a user followed a stale or mistyped link.

import { describe, it, expect, beforeAll, afterEach, vi } from 'vitest'
import { render, cleanup, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import type { ReactElement } from 'react'
import i18n from '../i18n' // real i18next instance — otherwise t() returns raw keys
import NotFound from './NotFound'
import styles from './NotFound.module.css'

beforeAll(async () => {
  window.matchMedia = window.matchMedia || ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia
  // The 404 page links into the knowledge page, which fetches on mount; stub it
  // so this test asserts routing, not the network.
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ result: [] }),
  }))
  // i18n picks zh-CN or en from the stored language / navigator; pin it so the
  // assertions below do not depend on the machine locale.
  await i18n.changeLanguage('zh-CN')
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/" element={<div>home page</div>} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </MemoryRouter> as ReactElement,
  )
}

describe('NotFound', () => {
  it('renders a real 404 state for an unmatched URL', async () => {
    renderAt('/settings/plugins')
    expect(await screen.findByText('404')).toBeTruthy()
    expect(document.body.textContent).toContain('页面不存在')
    // A leaked key would render as "notFound.title" instead of the copy.
    expect(document.body.textContent).not.toMatch(/notFound\./)
  })

  it('echoes the offending path so the user can see what went wrong', async () => {
    renderAt('/no/such/page')
    await screen.findByText('404')
    expect(document.body.textContent).toContain('/no/such/page')
  })

  it('offers a way out instead of a dead end', async () => {
    renderAt('/whatever')
    await screen.findByText('404')
    const hrefs = [...document.querySelectorAll(`.${styles.notfoundActions} a`)].map(a => a.getAttribute('href'))
    expect(hrefs).toContain('/')
    expect(hrefs.length).toBeGreaterThanOrEqual(2)
  })

  it('does not shadow a registered route', async () => {
    renderAt('/')
    expect(await screen.findByText('home page')).toBeTruthy()
    expect(document.body.textContent).not.toContain('404')
  })
})
