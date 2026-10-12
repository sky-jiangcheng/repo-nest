// Regenerate every icon derivative from the brand SSOT (build/icon.svg).
//
//   node scripts/generate-icons.mjs          # (re)generate all derivatives
//   node scripts/generate-icons.mjs --check  # verify derivatives are in sync
//
// build/icon.svg is the single source of truth for the Repotory brand mark.
// Everything else — the Wails appicon, the Windows .ico, the size ladder in
// build/icons/, the VS Code extension icon and the docs favicons — is a
// derived artifact. When the SSOT changes without regenerating, `--check`
// fails (wired into CI), so a rebrand can never ship half-applied.
import fs from 'node:fs'
import path from 'node:path'
import crypto from 'node:crypto'
import sharp from 'sharp'

const SIZES = [512, 256, 128, 64, 48, 32, 16]

const root = process.cwd()
const svgPath = path.join(root, 'build/icon.svg')
const outDir = path.join(root, 'build/icons')

// Favicon variant of the mark: same construction as the SSOT but with
// heavier strokes and no interior lines — at 16×16 a favicon reads by
// silhouette, and hairline strokes disappear.
const FAVICON_SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512" role="img" aria-label="Repotory">
  <title>Repotory</title>
  <defs>
    <linearGradient id="rn-bg" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#1F2C4D"/>
      <stop offset="1" stop-color="#0B1526"/>
    </linearGradient>
    <linearGradient id="rn-gold" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="0" y2="512">
      <stop offset="0" stop-color="#F7CE58"/>
      <stop offset="1" stop-color="#E0A63C"/>
    </linearGradient>
    <clipPath id="rn-round">
      <rect x="0" y="0" width="512" height="512" rx="104" ry="104"/>
    </clipPath>
  </defs>
  <g clip-path="url(#rn-round)">
    <rect x="0" y="0" width="512" height="512" fill="url(#rn-bg)"/>
    <g stroke="url(#rn-gold)" fill="none" stroke-linecap="round">
      <rect x="182" y="88" width="148" height="164" rx="34" stroke-width="52"/>
      <path d="M72 246A184 184 0 0 0 440 246" stroke-width="54"/>
      <path d="M122 256A134 134 0 0 0 390 256" stroke-width="44"/>
    </g>
  </g>
</svg>
`

// Every derived artifact: how it is produced, plus (for bitmaps) a pixel
// diff tolerance for --check. sharp upgrades can shift anti-aliased edges
// by a level, so the check compares pixel content with a small allowance
// instead of bytes — enough to catch a stale derivative, tolerant of
// renderer micro-drift.
const DERIVED = [
  { file: 'build/icons/icon-128.png', rel: 'ide/vscode/media/icon.png', copy: true },
  { file: 'build/icons/icon.ico', rel: 'docs/favicon.ico', copy: true },
  { file: 'build/icons/icon.ico', rel: 'web/public/favicon.ico', copy: true },
  { file: 'docs/favicon.svg', rel: 'web/public/favicon.svg', copy: true },
]

function sha(buf) {
  return crypto.createHash('sha256').update(buf).digest('hex')
}

async function renderPng(svg, size) {
  return sharp(svg).resize(size, size).png().toBuffer()
}

function buildIco(svg) {
  // ICO container: 6-byte header, one 16-byte directory entry per size,
  // then the embedded PNGs. Sizes >=256 are recorded as 0 per spec.
  const icoSizes = [256, 128, 64, 48, 32, 16]
  return Promise.all(icoSizes.map((s) => renderPng(svg, s))).then((bufs) => {
    const entries = bufs.map((buf, i) => ({ size: icoSizes[i], buf }))
    const headerSize = 6 + entries.length * 16
    let offset = headerSize
    let total = headerSize
    for (const e of entries) total += e.buf.length
    const ico = Buffer.alloc(total)
    ico.writeUInt16LE(0, 0)
    ico.writeUInt16LE(1, 2) // type: icon
    ico.writeUInt16LE(entries.length, 4)
    let at = 6
    for (const e of entries) {
      ico.writeUInt8(e.size >= 256 ? 0 : e.size, at)
      ico.writeUInt8(e.size >= 256 ? 0 : e.size, at + 1)
      ico.writeUInt16LE(1, at + 4) // planes
      ico.writeUInt16LE(32, at + 6) // bpp
      ico.writeUInt32LE(e.buf.length, at + 8)
      ico.writeUInt32LE(offset, at + 12)
      e.buf.copy(ico, offset)
      offset += e.buf.length
      at += 16
    }
    return ico
  })
}

async function pixelDiff(bufA, bufB) {
  // Both buffers are PNG encodings of the same dimensions from the same
  // encoder family. Compare raw pixels; return the share of pixels whose
  // channels differ beyond the allowance.
  const [a, b] = await Promise.all([
    sharp(bufA).raw().toBuffer({ resolveWithObject: true }),
    sharp(bufB).raw().toBuffer({ resolveWithObject: true }),
  ])
  if (a.info.width !== b.info.width || a.info.height !== b.info.height) {
    return 1 // dimension drift = totally stale
  }
  const ch = Math.min(a.info.channels, b.info.channels)
  let diff = 0
  const total = a.info.width * a.info.height
  for (let p = 0; p < total; p++) {
    for (let c = 0; c < ch; c++) {
      if (Math.abs(a.data[p * a.info.channels + c] - b.data[p * b.info.channels + c]) > 3) {
        diff++
        break
      }
    }
  }
  return diff / total
}

async function main() {
  const check = process.argv.includes('--check')
  const svg = fs.readFileSync(svgPath)

  if (!check) fs.mkdirSync(outDir, { recursive: true })

  const stale = []

  // 1. Size ladder + appicon + ico (from the SSOT)
  for (const size of SIZES) {
    const png = await renderPng(svg, size)
    const file = path.join(outDir, `icon-${size}.png`)
    if (check) {
      if (!fs.existsSync(file)) stale.push(`${file} (missing)`)
      else if ((await pixelDiff(png, fs.readFileSync(file))) > 0.005) stale.push(`${file} (pixel drift)`)
    } else {
      fs.writeFileSync(file, png)
      console.log(`  ✓ icon-${size}.png`)
    }
  }

  const appIcon = path.join(root, 'build/appicon.png')
  const appPng = await renderPng(svg, 512)
  if (check) {
    if (!fs.existsSync(appIcon)) stale.push(`${appIcon} (missing)`)
    else if ((await pixelDiff(appPng, fs.readFileSync(appIcon))) > 0.005) stale.push(`${appIcon} (pixel drift)`)
  } else {
    fs.writeFileSync(appIcon, appPng)
    console.log('  ✓ appicon.png (512x512 for Wails)')
  }

  const ico = await buildIco(svg)
  const icoPath = path.join(outDir, 'icon.ico')
  if (check) {
    if (!fs.existsSync(icoPath) || sha(ico) !== sha(fs.readFileSync(icoPath))) stale.push(`${icoPath} (stale)`)
  } else {
    fs.writeFileSync(icoPath, ico)
    console.log(`  ✓ icon.ico (6 sizes)`)
  }

  // 2. Cross-target derivatives (VS Code media, browser favicons)
  const faviconSvg = Buffer.from(FAVICON_SVG)
  if (check) {
    const favPath = path.join(root, 'docs/favicon.svg')
    if (!fs.existsSync(favPath) || sha(faviconSvg) !== sha(fs.readFileSync(favPath))) {
      stale.push(`${favPath} (stale)`)
    }
  } else {
    fs.writeFileSync(path.join(root, 'docs/favicon.svg'), faviconSvg)
    console.log('  ✓ docs/favicon.svg')
  }

  for (const d of DERIVED) {
    const gen = d.file.endsWith('.ico') ? ico : fs.readFileSync(path.join(root, d.file))
    const target = path.join(root, d.rel)
    if (check) {
      if (!fs.existsSync(target)) stale.push(`${target} (missing)`)
      else if (sha(gen) !== sha(fs.readFileSync(target))) stale.push(`${target} (stale)`)
    } else {
      fs.mkdirSync(path.dirname(target), { recursive: true })
      fs.writeFileSync(target, gen)
      console.log(`  ✓ ${d.rel}`)
    }
  }

  if (check) {
    if (stale.length) {
      console.error(`✗ ${stale.length} icon derivative(s) out of sync with build/icon.svg:`)
      for (const s of stale) console.error(`  - ${s}`)
      console.error('\nRun: node scripts/generate-icons.mjs')
      process.exit(1)
    }
    console.log('✓ all icon derivatives are in sync with build/icon.svg')
  } else {
    console.log(`\nAll icons generated (SSOT: build/icon.svg)`)
  }
}

main().catch((err) => {
  console.error('Failed to generate icons:', err)
  process.exit(1)
})
