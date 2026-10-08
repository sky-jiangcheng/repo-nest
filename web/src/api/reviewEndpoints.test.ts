// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'

const callMock = vi.fn()
vi.mock('./transport', () => ({ call: (...args: unknown[]) => callMock(...args) }))

import {
  approveWikiPage, cancelCompileJob, deleteCompiledPage, getCompileJob,
  listCompileJobs, listPendingWikiPages, listRejectedWikiPages,
  rejectWikiPage, startCompileJob,
} from './endpoints'

beforeEach(() => {
  callMock.mockReset()
  callMock.mockResolvedValue([])
})

describe('wiki review & compile-job endpoints', () => {
  it('routes each call to its binding with the right args', async () => {
    await listPendingWikiPages(7)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'ListPendingWikiPages', args: [7] })

    await listRejectedWikiPages(0)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'ListRejectedWikiPages', args: [0] })

    await approveWikiPage(12)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'ApproveWikiPage', args: [12] })

    await rejectWikiPage(12)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'RejectWikiPage', args: [12] })

    await deleteCompiledPage(12)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'DeleteCompiledPage', args: [12] })

    await startCompileJob(3, 10)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'StartCompileJob', args: [3, 10] })

    await cancelCompileJob(9)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'CancelCompileJob', args: [9] })
  })

  // The review tab dereferences these arrays to decide what to render; a null
  // from the binding would crash the page. Same degrade-to-empty discipline the
  // other list endpoints already hold.
  it('degrades a null list to empty rather than throwing in the UI', async () => {
    callMock.mockResolvedValue(null)
    expect(await listPendingWikiPages(0)).toEqual([])
    expect(await listRejectedWikiPages(0)).toEqual([])
    expect(await listCompileJobs(0, 10)).toEqual([])
    expect(await startCompileJob(1, 5)).toBe(0)
  })

  it('passes a job id through for polling and forwards null as-is', async () => {
    const job = { id: 4, status: 'running', notes_done: 1, notes_total: 3 }
    callMock.mockResolvedValue(job)
    expect(await getCompileJob(4)).toBe(job)
    callMock.mockResolvedValue(null)
    expect(await getCompileJob(4)).toBeNull()
  })
})
