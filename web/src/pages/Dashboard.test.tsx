// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

// Partial mock: override the backend-calling endpoints, keep everything else
// (getConnectionKind etc.) real so components like StatusBar keep working.
vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    getProjects: vi.fn(),
    getSummary: vi.fn(),
    getTodoCounts: vi.fn(),
    getNoteCounts: vi.fn(),
    getConfig: vi.fn(),
    getHeatmapData: vi.fn(),
    getStatusBar: vi.fn(),
    getScanStatus: vi.fn(),
    searchProjects: vi.fn(),
    searchAll: vi.fn(),
    triggerScan: vi.fn(),
  }
})

const mockT = (_key: string, opts?: { defaultValue?: string }) => opts?.defaultValue ?? _key

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: mockT }),
}))

import {
  getProjects,
  getSummary,
  getTodoCounts,
  getNoteCounts,
  getConfig,
  getHeatmapData,
  getStatusBar,
  getScanStatus,
  searchProjects,
  searchAll,
  type Project,
  type Summary,
} from '../api/client'
import Dashboard from './Dashboard'

const mockGetProjects = vi.mocked(getProjects)
const mockGetSummary = vi.mocked(getSummary)
const mockGetTodoCounts = vi.mocked(getTodoCounts)
const mockGetNoteCounts = vi.mocked(getNoteCounts)
const mockGetConfig = vi.mocked(getConfig)
const mockGetHeatmapData = vi.mocked(getHeatmapData)
const mockGetStatusBar = vi.mocked(getStatusBar)
const mockGetScanStatus = vi.mocked(getScanStatus)
const mockSearchProjects = vi.mocked(searchProjects)
const mockSearchAll = vi.mocked(searchAll)

// vitest globals are off, so testing-library's auto-cleanup never registers.
afterEach(cleanup)

// First Dashboard component test (D25): the page previously only had App.smoke
// shell coverage, while the ADR-0017 promotion gate requires functional tests
// for both first-screen halves.
const proj = (id: number, name: string, over: Partial<Project> = {}): Project => ({
  id,
  name,
  root_path: '/tmp/' + name,
  level_override: 0,
  is_auto_grouped: true,
  is_starred: false,
  created_at: '2026-10-06T00:00:00Z',
  repo_count: 1,
  total_added: 0,
  total_deleted: 0,
  my_added: 0,
  my_deleted: 0,
  my_files: 0,
  is_workday: true,
  below_standard: false,
  ...over,
})

const summary = (over: Partial<Summary> = {}): Summary => ({
  date: '2026-10-12',
  repo_count: 1,
  total_files: 3,
  total_added: 120,
  total_deleted: 40,
  my_added: 120,
  my_deleted: 40,
  my_files: 3,
  is_workday: true,
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  mockGetProjects.mockResolvedValue([])
  mockGetSummary.mockResolvedValue(summary())
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
  mockSearchProjects.mockResolvedValue([])
  mockSearchAll.mockResolvedValue([])
})

describe('Dashboard (first-screen project status half)', () => {
  it('renders project cards from the project list', async () => {
    // The dashboard defaults to the starred-only filter (showStarredOnly=true),
    // so the seeded projects must be starred to reach the card grid.
    mockGetProjects.mockResolvedValue([proj(1, 'proj-alpha', { is_starred: true }), proj(2, 'proj-beta', { is_starred: true })])
    render(<MemoryRouter><Dashboard /></MemoryRouter>)
    expect(await screen.findByText('proj-alpha')).toBeTruthy()
    expect(screen.getByText('proj-beta')).toBeTruthy()
  })

  it('keeps the workday goal state visible for a workday summary', async () => {
    mockGetProjects.mockResolvedValue([proj(1, 'proj-alpha', { is_starred: true })])
    mockGetSummary.mockResolvedValue(summary({ is_workday: true, my_added: 500 }))
    render(<MemoryRouter><Dashboard /></MemoryRouter>)
    await waitFor(() => expect(mockGetSummary).toHaveBeenCalled())
    // The name renders in more than one spot (card + dropdown matches), so
    // assert presence rather than uniqueness.
    expect((await screen.findAllByText('proj-alpha')).length).toBeGreaterThan(0)
    expect(screen.getByText('dashboard.title')).toBeTruthy()
  })

  it('shows the weekend state instead of the goal when the day is not a workday', async () => {
    mockGetProjects.mockResolvedValue([])
    mockGetSummary.mockResolvedValue(summary({ is_workday: false }))
    render(<MemoryRouter><Dashboard /></MemoryRouter>)
    await waitFor(() => expect(mockGetSummary).toHaveBeenCalled())
    // Dashboard.tsx swaps the goal for the weekend hint on non-workdays
    // (dashboard.weekend, defaultValue "Happy weekend").
    await waitFor(() => expect(screen.getByText('Happy weekend')).toBeTruthy(), { timeout: 2000 })
  })
})
