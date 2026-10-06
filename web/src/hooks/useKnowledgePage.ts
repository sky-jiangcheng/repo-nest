import { useState, useEffect, useMemo, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import {
  listAllNotes, listAllTags, searchAll, pinNote, importClaudeMemory, exportNoteAsMarkdown,
  getProjects,
  type NoteWithProject, type Project, type SearchHit,
} from '../api/client'
import { parseTags } from '../utils/markdown'
import { copyText } from '../utils/clipboard'
import { useDebouncedCallback } from './useDebouncedCallback'

export type KindFilter = 'all' | 'knowledge' | 'other'

/**
 * Encapsulates all data-fetching, search, and mutation logic for the
 * Knowledge page. The page component only handles rendering.
 */
export function useKnowledgePage() {
  const { t } = useTranslation()
  const [notes, setNotes] = useState<NoteWithProject[]>([])
  const [tags, setTags] = useState<string[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [hits, setHits] = useState<SearchHit[] | null>(null)
  const [kindFilter, setKindFilter] = useState<KindFilter>('all')
  const [activeTag, setActiveTag] = useState<string | null>(null)
  const [pinnedOnly, setPinnedOnly] = useState(false)
  const [importing, setImporting] = useState(false)
  const [message, setMessage] = useState('')
  const [newNotePicker, setNewNotePicker] = useState(false)
  const [askMode, setAskMode] = useState(false)
  const [exportingId, setExportingId] = useState<number | null>(null)
  const messageTimer = useRef<number | null>(null)
  const searchSeq = useRef(0)

  const flashMessage = useCallback((msg: string, ms = 3000) => {
    setMessage(msg)
    if (messageTimer.current) clearTimeout(messageTimer.current)
    messageTimer.current = window.setTimeout(() => setMessage(''), ms)
  }, [])

  useEffect(() => () => { if (messageTimer.current) clearTimeout(messageTimer.current) }, [])

  const fetchAll = useCallback(() => {
    setError('')
    Promise.all([listAllNotes(), listAllTags(), getProjects()])
      .then(([n, tg, ps]) => { setNotes(n); setTags(tg); setProjects(ps) })
      .catch((e: unknown) => { setError(e instanceof Error ? e.message : t('common.failed')) })
      .finally(() => setLoading(false))
  }, [t])

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { void fetchAll() }, [fetchAll])

  const runSearch = useDebouncedCallback((q: string, ask: boolean) => {
    if (!q.trim()) { setHits(null); return }
    const seq = ++searchSeq.current
    searchAll(q.trim())
      .then(h => { if (seq === searchSeq.current) { setHits(h); if (ask) setAskMode(true) } })
      .catch(() => { if (seq === searchSeq.current) { setHits([]); if (ask) setAskMode(true) } })
  }, 300)

  const handleSearchInput = useCallback((q: string) => {
    setQuery(q)
    runSearch(q, askMode)
  }, [runSearch, askMode])

  const handlePin = useCallback(async (id: number, pinned: boolean) => {
    setNotes(prev => prev.map(n => n.id === id ? { ...n, pinned: !pinned } : n))
    try { await pinNote(id, !pinned) } catch (e: unknown) {
      setNotes(prev => prev.map(n => n.id === id ? { ...n, pinned } : n))
      flashMessage(t('knowledge.pinFailed', { defaultValue: 'Failed to pin note' }) + (e instanceof Error ? ' ' + e.message : ''))
    }
  }, [flashMessage, t])

  const handleExport = useCallback(async (id: number) => {
    setExportingId(id)
    try {
      const md = await exportNoteAsMarkdown(id)
      if (!md) return
      await copyText(md)
      flashMessage(t('knowledge.copiedMd'))
    } catch (e) {
      flashMessage(t('knowledge.exportFailed') + (e instanceof Error ? e.message : t('common.unknownError')))
    } finally {
      setExportingId(null)
    }
  }, [flashMessage, t])

  const handleImport = useCallback(async () => {
    setImporting(true)
    try {
      const r = await importClaudeMemory()
      flashMessage(t('knowledge.importDone', { created: r.synced, updated: r.updated, skipped: r.skipped }), 4000)
      fetchAll()
    } catch (e) {
      flashMessage(t('knowledge.importFailed') + (e instanceof Error ? e.message : t('common.unknownError')), 4000)
    } finally {
      setImporting(false)
    }
  }, [flashMessage, fetchAll, t])

  const filtered = useMemo(() => {
    let list = notes
    if (kindFilter === 'knowledge') list = list.filter(n => n.kind === 'knowledge')
    else if (kindFilter === 'other') list = list.filter(n => n.kind !== 'knowledge')
    if (activeTag) list = list.filter(n => parseTags(n.tags).includes(activeTag))
    if (pinnedOnly) list = list.filter(n => n.pinned)
    return list
  }, [notes, kindFilter, activeTag, pinnedOnly])

  // Quick Note / Create Note / the project jump list must all work on a cold
  // start (zero notes), so this is derived from the scanned project list.
  // Deriving it from notes (the old behavior) deadlocked the first note: no
  // notes -> no projects to attach it to -> "No projects found" guard.
  const projectNames = useMemo(() => {
    const set = new Map<string, number>()
    projects.forEach(p => set.set(p.name, p.id))
    return Array.from(set.entries()).sort((a, b) => a[0].localeCompare(b[0]))
  }, [projects])

  const pinnedCount = useMemo(() => notes.filter(n => n.pinned).length, [notes])

  const recentNotes = useMemo(() => {
    return [...notes]
      .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
      .slice(0, 5)
  }, [notes])

  return {
    // State
    notes, tags, loading, error, query, hits, kindFilter, activeTag,
    pinnedOnly, importing, message, newNotePicker, askMode, exportingId,
    // Derived
    filtered, projectNames, pinnedCount, recentNotes,
    // Actions
    setQuery, setKindFilter, setActiveTag, setPinnedOnly, setNewNotePicker,
    handleSearchInput, handlePin, handleExport, handleImport, fetchAll, flashMessage,
  }
}
