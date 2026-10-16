// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent, within, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    searchAll: vi.fn(),
    getProjects: vi.fn(),
    listAllTags: vi.fn(),
  }
})

// Same stable-t convention as Home.test.tsx: components pass defaultValue
// inline, so the mock returns it verbatim (namespace prefixes are irrelevant
// under this mock).
const mockT = (_key: string, opts?: { defaultValue?: string }) => opts?.defaultValue ?? _key

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: mockT }),
}))

import { searchAll, getProjects, listAllTags, type Project, type SearchHit } from '../api/client'
import Search from './Search'
import { invalidateCache } from '../hooks/useApiData'

const mockSearch = vi.mocked(searchAll)
const mockGetProjects = vi.mocked(getProjects)
const mockListAllTags = vi.mocked(listAllTags)

const project = (over: Partial<Project>): Project => ({
  id: 1,
  name: 'Alpha',
  root_path: '/a',
  level_override: 0,
  is_auto_grouped: false,
  is_starred: false,
  created_at: '2026-10-01',
  repo_count: 2,
  total_added: 0,
  total_deleted: 0,
  my_added: 0,
  my_deleted: 0,
  my_files: 0,
  is_workday: true,
  below_standard: false,
  ...over,
})

const renderSearch = (initial = '/search') =>
  render(<MemoryRouter initialEntries={[initial]}><Search /></MemoryRouter>)

// vitest globals are off, so testing-library's auto-cleanup never registers.
afterEach(cleanup)

// useApiData's module-level cache (keyed 'projects:all' / 'tags:all') survives
// across tests in this file — bust it so every case mounts cold.
beforeEach(() => {
  vi.clearAllMocks()
  invalidateCache()
  mockSearch.mockResolvedValue([])
  mockGetProjects.mockResolvedValue([])
  mockListAllTags.mockResolvedValue([])
})

describe('Search page (ADR-0024 top-level global search)', () => {
  it('idle state recommends hot projects ranked by recent additions, starred as tiebreaker', async () => {
    mockGetProjects.mockResolvedValue([
      project({ id: 1, name: 'Quiet', my_added: 0, is_starred: false }),
      project({ id: 2, name: 'Busy', my_added: 120, is_starred: false }),
      project({ id: 3, name: 'Starred', my_added: 0, is_starred: true }),
    ])
    renderSearch()
    expect(await screen.findByText('Busy')).toBeTruthy()
    // my_added desc first; the starred project outranks the zero-activity tie.
    const hrefs = screen.getAllByRole('link').map(l => l.getAttribute('href'))
    expect(hrefs[0]).toBe('/project/2')
    expect(hrefs[1]).toBe('/project/3')
    expect(hrefs[2]).toBe('/project/1')
    // Idle is recommendations, not a search round-trip.
    expect(mockSearch).not.toHaveBeenCalled()
  })

  it('renders hot tag chips that fill the input and trigger the search', async () => {
    mockListAllTags.mockResolvedValue(['登录排查', '架构'])
    renderSearch()
    const chip = await screen.findByRole('button', { name: '登录排查' })
    fireEvent.click(chip)
    // Chip click sets the query; the debounced effect fires searchAll.
    await waitFor(() => expect(mockSearch).toHaveBeenCalledWith('登录排查'), { timeout: 2000 })
  })

  it('searches (debounced) and groups note/todo hits under labelled regions', async () => {
    mockSearch.mockResolvedValue([
      { type: 'note', id: 5, project_id: 2, project_name: 'Alpha', title: '登录排查笔记', snippet: '关于 <mark>登录</mark> 的记录', updated_at: '2026-10-12 10:00:00' },
      { type: 'todo', id: 9, project_id: 3, project_name: 'Beta', title: '修复登录超时', snippet: '', updated_at: '2026-10-12 10:00:00' },
    ] satisfies SearchHit[])
    renderSearch()
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: '登录' } })
    await waitFor(() => expect(mockSearch).toHaveBeenCalledWith('登录'), { timeout: 2000 })
    const notes = await screen.findByRole('region', { name: '笔记' })
    const todos = screen.getByRole('region', { name: '待办' })
    expect(within(notes).getByText('登录排查笔记')).toBeTruthy()
    expect(within(todos).getByText('修复登录超时')).toBeTruthy()
    // Both hit types use the shared global type-badge classes.
    expect(document.querySelector('.hit-type-note')).not.toBeNull()
    expect(document.querySelector('.hit-type-todo')).not.toBeNull()
  })

  it('shows project-name matches under the projects group', async () => {
    mockGetProjects.mockResolvedValue([project({ id: 7, name: 'repotory-core', my_added: 30 })])
    renderSearch()
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'repotory' } })
    await waitFor(() => expect(mockSearch).toHaveBeenCalledWith('repotory'), { timeout: 2000 })
    const group = await screen.findByRole('region', { name: '项目' })
    expect(group.textContent).toContain('repotory-core')
  })

  it('pre-fills and searches from the ?q= deep link', async () => {
    mockSearch.mockResolvedValue([
      { type: 'note', id: 5, project_id: 2, project_name: 'Alpha', title: '登录排查笔记', snippet: '', updated_at: '2026-10-12 10:00:00' },
    ] satisfies SearchHit[])
    renderSearch('/search?q=登录')
    expect(await screen.findByText('登录排查笔记')).toBeTruthy()
    expect(mockSearch).toHaveBeenCalledWith('登录')
    expect((screen.getByRole('searchbox') as HTMLInputElement).value).toBe('登录')
  })

  it('shows the no-results message when nothing matches', async () => {
    renderSearch()
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: '不存在' } })
    await waitFor(() => expect(mockSearch).toHaveBeenCalledWith('不存在'), { timeout: 2000 })
    // The text renders twice (visible message + aria-live mirror), so assert
    // on the list instead of findByText, which throws on multiple matches.
    const msgs = await screen.findAllByText('没有匹配的结果')
    expect(msgs.length).toBeGreaterThan(0)
  })
})
