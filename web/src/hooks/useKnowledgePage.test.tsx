// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'

vi.mock('../api/client', () => ({
  listAllNotes: vi.fn(),
  listAllTags: vi.fn(),
  getProjects: vi.fn(),
  searchAll: vi.fn(),
  pinNote: vi.fn(),
  importClaudeMemory: vi.fn(),
  exportNoteAsMarkdown: vi.fn(),
}))

// A single stable `t` identity: the hook's fetchAll depends on [t], so a
// fresh function per render (what a naive mock produces) would retrigger the
// fetch effect forever. Real react-i18next memoizes t.
const mockT = (_key: string, opts?: { defaultValue?: string }) => opts?.defaultValue ?? _key

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: mockT }),
}))

import { useKnowledgePage } from './useKnowledgePage'
import { getProjects, listAllNotes, listAllTags, type NoteWithProject, type Project } from '../api/client'

const mockListAllNotes = vi.mocked(listAllNotes)
const mockListAllTags = vi.mocked(listAllTags)
const mockGetProjects = vi.mocked(getProjects)

// The hook resolves through Promise microtasks; a nested act flush settles
// them without relying on timers.
const flush = async () => { await act(async () => {}) }

const proj = (id: number, name: string): Project => ({
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
})

const note = (id: number, projectId: number, projectName: string): NoteWithProject => ({
  id,
  project_id: projectId,
  title: 'n' + id,
  content: '',
  tags: '',
  kind: 'knowledge',
  pinned: false,
  source: '',
  sort_order: 0,
  created_at: '2026-10-06T00:00:00Z',
  updated_at: '2026-10-06T00:00:00Z',
  project_name: projectName,
  root_path: '/tmp/' + projectName,
})

beforeEach(() => {
  mockListAllNotes.mockReset().mockResolvedValue([])
  mockListAllTags.mockReset().mockResolvedValue([])
  mockGetProjects.mockReset().mockResolvedValue([])
})

describe('useKnowledgePage', () => {
  it('derives projectNames from the scanned project list on a cold start (no notes)', async () => {
    // Regression: projectNames used to be derived from notes, so an empty
    // knowledge base yielded an empty list and Quick Note deadlocked on
    // "No projects found" even though projects existed.
    mockGetProjects.mockResolvedValue([
      proj(2, '.openclaw-autoclaw'),
      proj(3, '.cargo'),
      proj(5, 'jiangcheng'),
    ])
    const { result } = renderHook(() => useKnowledgePage())
    await flush()

    expect(result.current.projectNames).toEqual([
      ['.cargo', 3],
      ['.openclaw-autoclaw', 2],
      ['jiangcheng', 5],
    ])
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBe('')
  })

  it('keeps the no-projects guard accurate when nothing was scanned', async () => {
    const { result } = renderHook(() => useKnowledgePage())
    await flush()

    expect(result.current.projectNames).toEqual([])
  })

  it('loads notes, tags and projects together and keeps them in sync on fetchAll', async () => {
    mockListAllNotes.mockResolvedValue([note(1, 3, '.cargo')])
    mockListAllTags.mockResolvedValue(['arch'])
    mockGetProjects.mockResolvedValue([proj(3, '.cargo')])
    const { result } = renderHook(() => useKnowledgePage())
    await flush()

    expect(result.current.notes).toHaveLength(1)
    expect(result.current.tags).toEqual(['arch'])
    expect(result.current.projectNames).toEqual([['.cargo', 3]])

    // fetchAll runs again after an import; the projects list refreshes too.
    mockGetProjects.mockResolvedValue([proj(3, '.cargo'), proj(4, 'new-repo')])
    await act(async () => { result.current.fetchAll() })
    await flush()

    expect(result.current.projectNames).toEqual([['.cargo', 3], ['new-repo', 4]])
  })

  it('surfaces a fetch error instead of pretending the library is empty', async () => {
    mockListAllNotes.mockRejectedValue(new Error('backend down'))
    const { result } = renderHook(() => useKnowledgePage())
    await flush()

    expect(result.current.error).toBe('backend down')
    expect(result.current.loading).toBe(false)
  })
})
