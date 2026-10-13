// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'

const callMock = vi.fn()
vi.mock('./transport', () => ({ call: (...args: unknown[]) => callMock(...args) }))

import {
  approveWikiPage, cancelCompileJob, deleteCompiledPage, diffWikiPageVersion, findMergeCandidates,
  getCompileJob, listCompileJobs, listPendingWikiPages, listRejectedWikiPages, listWikiPageVersions,
  mergeWikiPages, rejectWikiPage, runCommunitySummary, startCompileJob,
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
    expect(await listCompileJobs(0, '', 10)).toEqual([])
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

// ADR-0019 (merge), ADR-0020 (community synthesis), ADR-0021 (page versions).
// The contract that matters here is the binding name and argument ORDER: a merge
// called as (winner, loser) would absorb the wrong page and destroy it, and the
// Wails layer resolves arguments positionally.
describe('merge / community / page-version endpoints', () => {
  it('routes each call to its binding with the right args', async () => {
    await findMergeCandidates(7, 20)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'FindMergeCandidates', args: [7, 20] })

    await mergeWikiPages(3, 8)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'MergeWikiPages', args: [3, 8] })

    await runCommunitySummary(7)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'RunCommunitySummary', args: [7] })

    await listWikiPageVersions(11)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'ListWikiPageVersions', args: [11] })

    await diffWikiPageVersion(11, 4)
    expect(callMock).toHaveBeenLastCalledWith({ method: 'DiffWikiPageVersion', args: [11, 4] })
  })

  // Null discipline, per kind. Lists degrade to [] (the review page maps over
  // them), a diff degrades to '' (the viewer renders nothing rather than
  // "undefined"), and the two report objects pass through: a null merge/community
  // report is "the run said nothing", NOT "the run moved nothing".
  it('degrades a null list and diff, and forwards a null report as null', async () => {
    callMock.mockResolvedValue(null)
    expect(await listWikiPageVersions(11)).toEqual([])
    expect(await diffWikiPageVersion(11, 4)).toBe('')
    expect(await mergeWikiPages(3, 8)).toBeNull()
    expect(await runCommunitySummary(7)).toBeNull()
  })

  it('normalises a null candidate report to an empty one', async () => {
    callMock.mockResolvedValue(null)
    expect(await findMergeCandidates(7, 20)).toEqual({
      project_id: 7, pages: 0, pairs_scored: 0, pairs_possible: 0, candidates: [], truncated: false,
    })
  })

  it('passes a real report, version list and merge result through unchanged', async () => {
    const report = {
      project_id: 7, pages: 12, pairs_scored: 9, pairs_possible: 66,
      candidates: [{
        winner_id: 8, winner_slug: 'payment-gateway', loser_id: 3, loser_slug: 'gateway-payment',
        score: 4, signals: ['linked', 'title-overlap'], shared_notes: 2, project_id: 7,
      }],
      truncated: true,
    }
    callMock.mockResolvedValue(report)
    expect(await findMergeCandidates(7, 20)).toEqual(report)

    const versions = [{
      id: 4, page_id: 11, title: '支付网关', content: 'a', kind: 'concept',
      source: 'compiler', created_at: '2026-10-12 15:04:05',
    }]
    callMock.mockResolvedValue(versions)
    expect(await listWikiPageVersions(11)).toEqual(versions)

    const merged = {
      loser_id: 3, loser_slug: 'gateway-payment', winner_id: 8, winner_slug: 'payment-gateway',
      edges_redirected: 2, edges_dropped: 1, notes_reattached: 3, loser_previous_status: 'approved',
    }
    callMock.mockResolvedValue(merged)
    expect(await mergeWikiPages(3, 8)).toBe(merged)
  })
})
