// BrandMark.test.tsx — guards that the app UI uses the real brand mark.
//
// The regression this prevents: the navbar rendered a "▦" text glyph while
// every shipped artifact (app icon, favicon, VS Code extension) carried the
// navy-and-gold nest from build/icon.svg. Nothing failed — the glyph just
// looked like a placeholder next to a real icon. These assertions make the
// mark a hard dependency, so swapping it back out breaks a test rather than
// shipping silently.

import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import BrandMark from './BrandMark'

describe('BrandMark', () => {
  it('renders an inline SVG rather than a text glyph', () => {
    const { container } = render(<BrandMark />)
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    // The old navbar used a bare "▦" character, which inherits font metrics
    // and has no geometry of its own.
    expect(svg!.querySelectorAll('rect, path').length).toBeGreaterThan(3)
  })

  it('carries the brand palette, not theme colours', () => {
    const { container } = render(<BrandMark />)
    const html = container.innerHTML
    // Navy tile and gold mark — the two brand colours.
    expect(html).toContain('#1F2C4D')
    expect(html).toContain('#F7CE58')
  })

  it('is hidden from assistive tech unless given a title', () => {
    const { container, rerender } = render(<BrandMark />)
    // Adjacent text already names the product, so the mark is decorative by
    // default — announcing it would double the name for screen readers.
    expect(container.querySelector('svg')!.getAttribute('aria-hidden')).toBe('true')

    rerender(<BrandMark title="RepoNest" />)
    const named = screen.getByRole('img', { name: 'RepoNest' })
    expect(named).toBeTruthy()
    expect(named.getAttribute('aria-hidden')).toBeNull()
  })

  it('gives each instance unique gradient ids', () => {
    const { container } = render(
      <>
        <BrandMark />
        <BrandMark />
      </>,
    )
    const ids = [...container.querySelectorAll('linearGradient')].map((g) => g.id)
    // Duplicate ids would make every instance resolve url(#id) to the first
    // one in the document, so the second mark would take the first's colours.
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('drops the interior card lines at small sizes', () => {
    const { container: big } = render(<BrandMark size={48} />)
    const bigPaths = big.querySelectorAll('path').length
    const { container: small } = render(<BrandMark size={16} />)
    const smallPaths = small.querySelectorAll('path').length
    // Below ~20px the two interior lines close into a smudge, which is why
    // generate-icons.mjs has a separate hairline-free favicon variant.
    expect(smallPaths).toBeLessThan(bigPaths)
  })
})
