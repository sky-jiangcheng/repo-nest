import { useState, useEffect, type Dispatch, type SetStateAction } from 'react'
import type { NoteDraft } from '../components/notes/NoteEditor'

const emptyDraft: NoteDraft = { content: '', title: '', tags: '', kind: 'knowledge' }

function draftKey(projectId: number) {
  return `repotory-note-draft-${projectId}`
}

// Spelling is intentional: this names the key written by builds before the
// GitBoard -> Repotory rename, and loadDraft() moves its value over.
function legacyDraftKey(projectId: number) {
  return `gitboard-note-draft-${projectId}`
}

function loadDraft(projectId: number): NoteDraft {
  try {
    let raw = localStorage.getItem(draftKey(projectId))
    if (!raw) {
      raw = localStorage.getItem(legacyDraftKey(projectId))
      if (raw) {
        localStorage.setItem(draftKey(projectId), raw)
        localStorage.removeItem(legacyDraftKey(projectId))
      }
    }
    if (raw) {
      const parsed = JSON.parse(raw)
      if (
        typeof parsed === 'object' && parsed !== null &&
        typeof parsed.content === 'string' &&
        typeof parsed.title === 'string' &&
        typeof parsed.tags === 'string' &&
        typeof parsed.kind === 'string'
      ) {
        return {
          content: parsed.content,
          title: parsed.title,
          tags: parsed.tags,
          kind: parsed.kind,
          pinned: typeof parsed.pinned === 'boolean' ? parsed.pinned : undefined,
        }
      }
    }
  } catch (e: unknown) { console.error('[useNoteDraft] loadDraft failed:', e) }
  return { ...emptyDraft }
}

/**
 * useNoteDraft manages the draft state for the current note editor, persisting
 * it to localStorage keyed by projectId. Supports migration from the old
 * `repotory-` key prefix.
 */
export function useNoteDraft(projectId: number): [NoteDraft, Dispatch<SetStateAction<NoteDraft>>, () => void] {
  const [draft, setDraft] = useState<NoteDraft>(() => loadDraft(projectId))

  useEffect(() => {
    try { localStorage.setItem(draftKey(projectId), JSON.stringify(draft)) } catch { /* ignore */ }
  }, [draft, projectId])

  const clearDraft = () => {
    setDraft({ ...emptyDraft })
    try { localStorage.removeItem(draftKey(projectId)) } catch { /* ignore */ }
  }

  return [draft, setDraft, clearDraft]
}
