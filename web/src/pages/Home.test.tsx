// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent, within, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

// Partial mock: override the backend-calling endpoints, keep everything else
// (getConnectionKind etc.) real so components like StatusBar keep working.
vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    listRecentHandoffs: vi.fn(),
    searchAll: vi.fn(),
    getProjects: vi.fn(),
    getSummary: vi.fn(),
    getTodoCounts: vi.fn(),
    getNoteCounts: vi.fn(),
    getConfig: vi.fn(),
    getHeatmapData: vi.fn(),
    getStatusBar: vi.fn(),
    getScanStatus: vi.fn(),
  }
})

// Same stable-t convention as useKnowledgePage.test.tsx: components pass
// defaultValue inline, so the mock returns it verbatim.
const mockT = (_key: string, opts?: { defaultValue?: string }) => opts?.defaultValue ?? _key

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: mockT }),
}))

import {
  listRecentHandoffs,
  searchAll,
  getProjects,
  getSummary,
  getTodoCounts,
  getNoteCounts,
  getConfig,
  getHeatmapData,
  getStatusBar,
  getScanStatus,
  type HandoffEntry,
  type Summary,
} from '../api/client'
import Home from './Home'

const mockList = vi.mocked(listRecentHandoffs)
const mockSearch = vi.mocked(searchAll)
const mockGetProjects = vi.mocked(getProjects)
const mockGetSummary = vi.mocked(getSummary)
const mockGetTodoCounts = vi.mocked(getTodoCounts)
const mockGetNoteCounts = vi.mocked(getNoteCounts)
const mockGetConfig = vi.mocked(getConfig)
const mockGetHeatmapData = vi.mocked(getHeatmapData)
const mockGetStatusBar = vi.mocked(getStatusBar)
const mockGetScanStatus = vi.mocked(getScanStatus)

const emptySummary: Summary = {
  date: '2026-10-12',
  repo_count: 0,
  total_files: 0,
  total_added: 0,
  total_deleted: 0,
  my_added: 0,
  my_deleted: 0,
  my_files: 0,
  is_workday: true,
}

const entry = (over: Partial<HandoffEntry>): HandoffEntry => ({
  note_id: 1,
  project_id: 2,
  project_name: 'Alpha',
  title: 'Session Handoff 2026-10-12 15:04',
  updated_at: '2026-10-12 15:04:05',
  auto_captured: false,
  ...over,
})

const renderHome = () => render(<MemoryRouter><Home /></MemoryRouter>)

// vitest globals are off, so testing-library's auto-cleanup never registers.
afterEach(cleanup)

// The hook resolves through Promise microtasks; a waitFor flush settles them
// without relying on timers.
const flush = async () => { await waitFor(() => {}) }

beforeEach(() => {
  vi.clearAllMocks()
  // Memory ring.
  mockList.mockResolvedValue([])
  mockSearch.mockResolvedValue([])
  // Embedded Dashboard panel — same shapes the real endpoints produce after
  // their own null fallbacks.
  mockGetProjects.mockResolvedValue([])
  mockGetSummary.mockResolvedValue(emptySummary)
  mockGetTodoCounts.mockResolvedValue([])
  mockGetNoteCounts.mockResolvedValue([])
  mockGetConfig.mockResolvedValue({ config: {}, scan_roots: [] })
  mockGetHeatmapData.mockResolvedValue({ days: [] })
  mockGetStatusBar.mockResolvedValue({
    current_time: '',
    last_commit_time: '',
    last_commit_repo: '',
    last_commit_branch: '',
    last_commit_msg: '',
  })
  mockGetScanStatus.mockResolvedValue({ running: false, backfilling: false, message: '', progress: 0, total: 0 })
})

describe('Home memory ring (D25 first screen)', () => {
  it('renders recent handoffs with project name and the auto-captured badge', async () => {
    mockList.mockResolvedValue([
      entry({ note_id: 1, auto_captured: true }),
      entry({ note_id: 2, project_name: 'Beta', title: 'Session Handoff 2026-10-11 09:30', auto_captured: false }),
    ])
    renderHome()
    expect(await screen.findByText('Session Handoff 2026-10-12 15:04')).toBeTruthy()
    expect(screen.getByText('Alpha')).toBeTruthy()
    expect(screen.getByText('Session Handoff 2026-10-11 09:30')).toBeTruthy()
    // Exactly one auto badge: the manual entry must not be flagged.
    expect(screen.getAllByText('自动')).toHaveLength(1)
  })

  it('shows the empty hint when no handoff exists', async () => {
    renderHome()
    expect(await screen.findByText('还没有 handoff——会话结束或手动捕捉后会出现在这里')).toBeTruthy()
  })

  it('searches globally (debounced) and renders note/todo hits', async () => {
    mockSearch.mockResolvedValue([
      { type: 'note', id: 5, project_id: 2, project_name: 'Alpha', title: '登录排查笔记', snippet: '关于 <mark>登录</mark> 的记录', updated_at: '2026-10-12 10:00:00' },
      { type: 'todo', id: 9, project_id: 3, project_name: 'Beta', title: '修复登录超时', snippet: '', updated_at: '2026-10-12 10:00:00' },
    ])
    renderHome()
    await flush()
    // Scope to the ring section: the embedded Dashboard carries a second
    // searchbox (ProjectSearchDropdown) with the same accessible name. The
    // ring is the first region carrying this label in DOM order.
    const ring = screen.getAllByRole('region', { name: '记忆环' })[0]
    fireEvent.change(within(ring).getByRole('searchbox', { name: '全局检索' }), { target: { value: '登录' } })
    // Debounce (150ms) + fetch settle.
    await waitFor(() => expect(mockSearch).toHaveBeenCalledWith('登录'), { timeout: 2000 })
    expect(await screen.findByText('登录排查笔记')).toBeTruthy()
    expect(screen.getByText('修复登录超时')).toBeTruthy()
    // Both hit types get their shared.css type chip.
    expect(document.querySelector('.hit-type-note')).not.toBeNull()
    expect(document.querySelector('.hit-type-todo')).not.toBeNull()
  })

  it('clears results when the query is emptied', async () => {
    mockSearch.mockResolvedValue([
      { type: 'note', id: 5, project_id: 2, project_name: 'Alpha', title: '登录排查笔记', snippet: '', updated_at: '2026-10-12 10:00:00' },
    ])
    renderHome()
    await flush()
    const box = within(screen.getByRole('region', { name: '记忆环' })).getByRole('searchbox', { name: '全局检索' })
    fireEvent.change(box, { target: { value: '登录' } })
    await screen.findByText('登录排查笔记')
    fireEvent.change(box, { target: { value: '' } })
    await waitFor(() => expect(screen.queryByText('登录排查笔记')).toBeNull(), { timeout: 2000 })
  })

  it('keeps the project status panel mounted beside the ring (ADR-0017)', async () => {
    renderHome()
    await flush()
    // Dashboard renders its (visually hidden) page title; its presence proves
    // the status overview is on the same screen, not hidden or routed away.
    expect(screen.getAllByText('dashboard.title').length).toBeGreaterThan(0)
  })
})
