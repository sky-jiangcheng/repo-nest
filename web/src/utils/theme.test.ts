import { describe, it, expect, beforeEach, vi } from 'vitest'
import { getEffectiveTheme, getStoredTheme, storeTheme, applyTheme } from './theme'

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {}
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => { store[key] = value },
    removeItem: (key: string) => { delete store[key] },
    clear: () => { store = {} },
  }
})()

Object.defineProperty(globalThis, 'localStorage', { value: localStorageMock })

// Mock matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: query === '(prefers-color-scheme: dark)' ? false : false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

beforeEach(() => {
  localStorageMock.clear()
  document.documentElement.removeAttribute('data-theme')
})

describe('getEffectiveTheme', () => {
  it('returns light for light mode', () => {
    expect(getEffectiveTheme('light')).toBe('light')
  })

  it('returns dark for dark mode', () => {
    expect(getEffectiveTheme('dark')).toBe('dark')
  })

  it('returns system preference for system mode', () => {
    // matchMedia mock returns false for dark, so system = light
    expect(getEffectiveTheme('system')).toBe('light')
  })
})

describe('getStoredTheme', () => {
  it('returns system as default', () => {
    expect(getStoredTheme()).toBe('system')
  })

  it('returns stored theme', () => {
    localStorage.setItem('repotory-theme', 'dark')
    expect(getStoredTheme()).toBe('dark')
  })

  it('migrates legacy theme key', () => {
    localStorage.setItem('gitboard-theme', 'light')
    expect(getStoredTheme()).toBe('light')
    expect(localStorage.getItem('repotory-theme')).toBe('light')
    expect(localStorage.getItem('gitboard-theme')).toBeNull()
  })

  it('returns system for invalid stored value', () => {
    localStorage.setItem('repotory-theme', 'invalid')
    expect(getStoredTheme()).toBe('system')
  })
})

describe('storeTheme', () => {
  it('stores theme in localStorage', () => {
    storeTheme('dark')
    expect(localStorage.getItem('repotory-theme')).toBe('dark')
  })

  it('overwrites previous value', () => {
    storeTheme('light')
    storeTheme('dark')
    expect(localStorage.getItem('repotory-theme')).toBe('dark')
  })
})

describe('applyTheme', () => {
  it('sets data-theme attribute for light', () => {
    applyTheme('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
  })

  it('sets data-theme attribute for dark', () => {
    applyTheme('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })

  it('resolves system theme', () => {
    applyTheme('system')
    const attr = document.documentElement.getAttribute('data-theme')
    expect(['light', 'dark']).toContain(attr)
  })
})
