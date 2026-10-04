export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'reponest-theme'
// Spelling is intentional: this names the key written by builds before the
// GitBoard -> RepoNest rename, and migrateLegacyTheme() moves its value over.
// Renaming it would silently reset everyone's theme on upgrade.
const LEGACY_STORAGE_KEY = 'gitboard-theme'

function migrateLegacyTheme() {
  try {
    const legacy = localStorage.getItem(LEGACY_STORAGE_KEY)
    if (legacy !== null && localStorage.getItem(STORAGE_KEY) === null) {
      localStorage.setItem(STORAGE_KEY, legacy)
      localStorage.removeItem(LEGACY_STORAGE_KEY)
    }
  } catch {
    // ignore
  }
}

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function getEffectiveTheme(mode: ThemeMode): 'light' | 'dark' {
  if (mode === 'system') return getSystemTheme()
  return mode
}

export function getStoredTheme(): ThemeMode {
  migrateLegacyTheme()
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw === 'light' || raw === 'dark' || raw === 'system') return raw
  } catch {
    // ignore
  }
  return 'system'
}

export function storeTheme(mode: ThemeMode) {
  try {
    localStorage.setItem(STORAGE_KEY, mode)
  } catch {
    // ignore
  }
}

export function applyTheme(mode: ThemeMode) {
  const effective = getEffectiveTheme(mode)
  document.documentElement.setAttribute('data-theme', effective)
}

/**
 * Read a design token's computed value from the document root.
 *
 * Canvas-rendered UI (Chart.js) and inline SVG attributes cannot use CSS
 * classes, so they need the token's real value — including the dark-theme
 * override. Reading it from the cascade (rather than hardcoding a hex per
 * theme) is what keeps those surfaces in sync with the stylesheet.
 * Returns '' when the token is unknown or there is no DOM.
 */
export function cssVar(name: string): string {
  if (typeof window === 'undefined') return ''
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

/** Current effective theme, read from the attribute applyTheme() sets. */
export function getCurrentTheme(): 'light' | 'dark' {
  if (typeof document === 'undefined') return 'light'
  return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light'
}

export function listenSystemTheme(callback: (theme: 'light' | 'dark') => void) {
  const mql = window.matchMedia('(prefers-color-scheme: dark)')
  const handler = (e: MediaQueryListEvent | MediaQueryList) => {
    callback(e.matches ? 'dark' : 'light')
  }
  mql.addEventListener('change', handler)
  return () => mql.removeEventListener('change', handler)
}
