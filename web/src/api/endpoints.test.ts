// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'

const callMock = vi.fn()
vi.mock('./transport', () => ({ call: (...args: unknown[]) => callMock(...args) }))

import {
  askAIWithEvidence,
  captureClaudeHandoff,
  fileAnswerAsPage,
  getProjectDetail,
  getProjects,
  searchAll,
  searchNotes,
  toggleStar,
} from './endpoints'
import type { Evidence } from './types'

beforeEach(() => {
  callMock.mockReset()
  callMock.mockResolvedValue([])
})

describe('endpoint routing contract', () => {
  it('searchAll routes to the SearchAll binding and the /search path', async () => {
    await searchAll('登录 排查')
    expect(callMock).toHaveBeenCalledWith({
      method: 'SearchAll',
      args: ['登录 排查'],
      path: '/search?q=%E7%99%BB%E5%BD%95%20%E6%8E%92%E6%9F%A5',
    })
  })

  it('searchNotes routes to the SearchNotes binding', async () => {
    await searchNotes('fts query')
    expect(callMock).toHaveBeenCalledWith({
      method: 'SearchNotes',
      args: ['fts query'],
      path: '/notes/search?q=fts%20query',
    })
  })

  it('getProjects encodes date and starred params into the query string', async () => {
    await getProjects('2026-08-01', true)
    expect(callMock).toHaveBeenCalledWith({
      method: 'GetProjects',
      args: ['2026-08-01', true],
      path: '/projects?date=2026-08-01&starred=1',
    })
  })

  it('toggleStar POSTs to the star path and unwraps a bare boolean', async () => {
    callMock.mockResolvedValueOnce(true)
    const starred = await toggleStar(7)
    expect(callMock).toHaveBeenCalledWith({
      method: 'ToggleStar',
      args: [7],
      path: '/projects/7/star',
      init: { method: 'POST' },
    })
    expect(starred).toBe(true)
  })

  it('getProjectDetail routes to the detail path with the project id', async () => {
    callMock.mockResolvedValueOnce({ id: 3 })
    await getProjectDetail(3)
    expect(callMock).toHaveBeenCalledWith({
      method: 'GetProjectDetail',
      args: [3],
      path: '/projects/3',
    })
  })

  it('captureClaudeHandoff POSTs per-project and returns the handoff result', async () => {
    const result = { note_id: 12, title: 'Session Handoff', tags: 'handoff, auto-captured' }
    callMock.mockResolvedValueOnce(result)
    const got = await captureClaudeHandoff(4)
    expect(callMock).toHaveBeenCalledWith({
      method: 'CaptureClaudeHandoff',
      args: [4],
      path: '/project/4/capture-claude-handoff',
      init: { method: 'POST' },
    })
    expect(got).toBe(result)
  })
})

const evidenceFixture: Evidence = {
  items: [{ ref: 'P1', type: 'page', id: 7, title: 'Payment Gateway retry policy', kind: 'entity', snippet: '...', rank: 1 }],
  dropped: 0,
  elapsed_ms: 3,
  truncated: false,
}

describe('evidence endpoints (ADR-0014 M6-W2)', () => {
  it('askAIWithEvidence routes to the AskAIWithEvidence binding', async () => {
    callMock.mockResolvedValue({ reply: 'because', evidence: evidenceFixture })
    const res = await askAIWithEvidence(12, 'why retries')
    expect(callMock).toHaveBeenCalledWith({ method: 'AskAIWithEvidence', args: [12, 'why retries'] })
    expect(res.reply).toBe('because')
    expect(res.evidence.items[0].ref).toBe('P1')
  })

  it('askAIWithEvidence degrades to an empty evidence set rather than null', async () => {
    // The panel dereferences evidence.items to decide what to show and what may be
    // filed; a null there is a blank tab and a dead button with no explanation.
    callMock.mockResolvedValue(null)
    const res = await askAIWithEvidence(12, 'why retries')
    expect(res.evidence.items).toEqual([])
  })

  it('fileAnswerAsPage forwards the retrieval the answer came from, plus its refs', async () => {
    callMock.mockResolvedValue(31)
    const id = await fileAnswerAsPage(12, 'why retries', 'Because [P1].', evidenceFixture, ['P1'])
    expect(callMock).toHaveBeenCalledWith({
      method: 'FileAnswerAsPage',
      args: [12, 'why retries', 'Because [P1].', evidenceFixture, ['P1']],
    })
    expect(id).toBe(31)
  })
})
