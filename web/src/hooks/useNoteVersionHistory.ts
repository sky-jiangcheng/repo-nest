import { useState, useRef } from 'react'
import type { NoteVersion } from '../api/client'

interface VersionHistoryState {
  versionHistory: NoteVersion[] | null
  currentNoteId: number | null
  restoringId: number | null
  diffText: string | null
}

interface VersionHistoryHandle extends VersionHistoryState {
  openVersionHistory: (noteId: number) => Promise<void>
  handleRestoreVersion: (versionId: number) => Promise<void>
  handleShowDiff: (versionId: number) => Promise<void>
  closeVersionHistory: () => void
}

/**
 * useNoteVersionHistory manages the version history panel state and its
 * async operations (list / restore / diff). Callers must pass in their own
 * `run` wrapper to integrate with the ErrorBanner retry mechanism.
 */
export function useNoteVersionHistory(
  run: (op: () => Promise<void>, errMsg: string) => Promise<boolean>
): VersionHistoryHandle {
  const [versionHistory, setVersionHistory] = useState<NoteVersion[] | null>(null)
  const [currentNoteId, setCurrentNoteId] = useState<number | null>(null)
  const [restoringId, setRestoringId] = useState<number | null>(null)
  const [diffText, setDiffText] = useState<string | null>(null)
  // Use a ref-based tuple as cache key to avoid string-prefix collision
  // (a diff text itself could start with "123-456\n...").
  const diffCacheKeyRef = useRef<[number, number] | null>(null)

  const openVersionHistory = async (noteId: number) => {
    if (currentNoteId === noteId && versionHistory !== null) {
      setVersionHistory(null)
      setCurrentNoteId(null)
      setDiffText(null)
      diffCacheKeyRef.current = null
      return
    }
    setCurrentNoteId(noteId)
    setVersionHistory(null)
    setDiffText(null)
    diffCacheKeyRef.current = null
    await run(async () => {
      const { listNoteVersions } = await import('../api/client')
      setVersionHistory(await listNoteVersions(noteId))
    }, 'Failed to load version history')
  }

  const handleRestoreVersion = async (versionId: number) => {
    if (currentNoteId === null) return
    setRestoringId(versionId)
    await run(async () => {
      const { restoreNoteVersion } = await import('../api/client')
      await restoreNoteVersion(currentNoteId, versionId)
      setVersionHistory(null)
      setCurrentNoteId(null)
      diffCacheKeyRef.current = null
    }, 'Failed to restore version')
    setRestoringId(null)
  }

  const handleShowDiff = async (versionId: number) => {
    if (currentNoteId === null) return
    const key: [number, number] = [currentNoteId, versionId]
    if (diffCacheKeyRef.current !== null &&
      diffCacheKeyRef.current[0] === key[0] && diffCacheKeyRef.current[1] === key[1]) {
      setDiffText(null)
      diffCacheKeyRef.current = null
      return
    }
    await run(async () => {
      const { diffNoteVersions } = await import('../api/client')
      const diff = await diffNoteVersions(currentNoteId, versionId)
      setDiffText(diff)
      diffCacheKeyRef.current = key
    }, 'Failed to load diff')
  }

  const closeVersionHistory = () => {
    setVersionHistory(null)
    setCurrentNoteId(null)
    setDiffText(null)
    diffCacheKeyRef.current = null
  }

  return {
    versionHistory, currentNoteId, restoringId, diffText,
    openVersionHistory, handleRestoreVersion, handleShowDiff, closeVersionHistory,
  }
}
