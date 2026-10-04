// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { call, checkHealth, getConnectionKind, subscribeConnection } from './transport'

// transport.ts registers online/offline listeners at import time; each test
// runs in a fresh jsdom so the global window is available.

beforeEach(() => {
  delete (window as unknown as { go?: unknown }).go
  vi.stubGlobal('fetch', vi.fn())
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('call routing', () => {
  it('routes through the Wails binding when window.go.app.App exists', async () => {
    const appMethod = vi.fn().mockResolvedValue({ ok: true })
    ;(window as unknown as { go: { app: { App: { Health: typeof appMethod } } } }).go = {
      app: { App: { Health: appMethod } },
    }
    const result = await call<{ ok: boolean }>({ method: 'Health', path: '/health' })
    expect(appMethod).toHaveBeenCalled()
    expect(result).toEqual({ ok: true })
  })

  it('falls back to the /api/rpc bridge when Wails is absent', async () => {
    // Browser/standalone mode posts every binding to one JSON-RPC endpoint
    // (httpapi /api/rpc), so there is no per-method REST path — `path` is
    // carried for the desktop/Wails call signature only.
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ result: { items: [1] } }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const result = await call<{ items: number[] }>({ method: 'ListProjects', args: [], path: '/projects' })
    expect(fetchMock).toHaveBeenCalledWith('/api/rpc', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ method: 'ListProjects', args: [] }),
    })
    expect(result).toEqual({ items: [1] })
  })

  it('throws the backend error message on HTTP failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Server Error',
      json: async () => ({ error: 'boom' }),
    }))
    await expect(call({ method: 'X', path: '/x' })).rejects.toThrow('boom')
  })
})

describe('connection health', () => {
  it('starts as ok', () => {
    expect(getConnectionKind()).toBe('ok')
  })

  it('marks backend-down when the Wails Health call rejects', async () => {
    ;(window as unknown as { go: { app: { App: { Health: () => Promise<never> } } } }).go = {
      app: { App: { Health: () => Promise.reject(new Error('no backend')) } },
    }
    const kinds: string[] = []
    subscribeConnection(k => kinds.push(k))
    const ok = await checkHealth()
    expect(ok).toBe(false)
    expect(kinds).toContain('backend-down')
  })

  it('treats a healthy response as ok', async () => {
    ;(window as unknown as { go: { app: { App: { Health: () => Promise<{ ok: boolean }> } } } }).go = {
      app: { App: { Health: () => Promise.resolve({ ok: true }) } },
    }
    const ok = await checkHealth()
    expect(ok).toBe(true)
    expect(getConnectionKind()).toBe('ok')
  })
})