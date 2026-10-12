// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'

const callMock = vi.fn()
vi.mock('./transport', () => ({ call: (...args: unknown[]) => callMock(...args) }))

import { listRecentHandoffs } from './memoryEndpoints'

beforeEach(() => {
  callMock.mockReset()
  callMock.mockResolvedValue([])
})

describe('memory endpoint routing contract', () => {
  it('listRecentHandoffs routes to the ListRecentHandoffs binding with the limit', async () => {
    await listRecentHandoffs(10)
    expect(callMock).toHaveBeenCalledWith({
      method: 'ListRecentHandoffs',
      args: [10],
      path: '/handoffs/recent',
    })
  })

  it('listRecentHandoffs defaults the limit and unwraps null to an empty array', async () => {
    callMock.mockResolvedValueOnce(null)
    const got = await listRecentHandoffs()
    expect(callMock).toHaveBeenCalledWith(
      expect.objectContaining({ method: 'ListRecentHandoffs', args: [10] }),
    )
    expect(got).toEqual([])
  })
})
