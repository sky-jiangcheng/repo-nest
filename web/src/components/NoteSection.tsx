import { useState, useEffect, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import {
  listNotes, createNoteWithMeta, updateNoteFull, deleteNote, pinNote, moveNote,
  getProjects,
  type Note, type Project,
} from '../api/client'
import { renderMarkdown } from '../utils/markdown'
import { useApiData } from '../hooks/useApiData'
import { useConfirmClick } from '../hooks/useConfirmClick'
import { useNoteMutations } from '../hooks/useNoteMutations'
import { useNoteDraft } from '../hooks/useNoteDraft'
import { useNoteVersionHistory } from '../hooks/useNoteVersionHistory'
import { useFilteredNotes } from '../hooks/useFilteredNotes'
import type { KindFilter } from '../types/kind'
import NoteEditor, { type NoteDraft } from './notes/NoteEditor'
import NoteFilterBar from './notes/NoteFilterBar'
import VersionHistoryPanel from './notes/VersionHistoryPanel'
import ErrorBanner from './ErrorBanner'
import Icon from './Icon'
import s from './NoteSection.module.css'

interface Props {
  projectId: number
  autoNew?: boolean
}

function NoteSection({ projectId, autoNew = false }: Props) {
  const { t } = useTranslation()
  const [notes, setNotes] = useState<Note[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editingId, setEditingId] = useState<number | null>(null)
  const [filter, setFilter] = useState<KindFilter>('all')
  const [saving, setSaving] = useState(false)

  const [draft, setDraft, clearDraft] = useNoteDraft(projectId)
  const { data: projectsData } = useApiData(() => getProjects(undefined, false), [], { cacheKey: 'projects:all' })
  const projects = projectsData ?? []

  const { run, retryLast } = useNoteMutations(setError)
  const vh = useNoteVersionHistory(run)

  // Sequence guard: when the project switches (or a refetch races a refetch),
  // a late response for an older query must not overwrite the current list —
  // or flip the loading/error state of a request that is no longer current.
  const fetchSeqRef = useRef(0)
  const fetchNotes = useCallback(() => {
    const seq = ++fetchSeqRef.current
    listNotes(projectId)
      .then(notes => { if (fetchSeqRef.current === seq) setNotes(notes) })
      .catch(e => {
        // A failed load must not masquerade as the "no notes yet" empty state.
        if (fetchSeqRef.current === seq) setError(`${t('project.loadFailed')}: ${e instanceof Error ? e.message : ''}`)
      })
      .finally(() => { if (fetchSeqRef.current === seq) setLoading(false) })
  }, [projectId, t])

  useEffect(() => { fetchNotes() }, [fetchNotes])

  const [isNew, setIsNew] = useState(false)
  const [editDraft, setEditDraft] = useState<NoteDraft>({ ...draft })

  useEffect(() => {
    if (autoNew) {
      setIsNew(true) // eslint-disable-line react-hooks/set-state-in-effect
      setEditingId(null)
    }
  }, [autoNew])

  const filteredNotes = useFilteredNotes(notes, filter)

  const startNew = () => {
    setIsNew(true)
    setEditingId(null)
  }

  const handleCreate = async () => {
    if (!draft.content.trim()) return
    setSaving(true)
    await run(async () => {
      await createNoteWithMeta(projectId, draft.content.trim(), {
        title: draft.title.trim(),
        tags: draft.tags.trim(),
        kind: draft.kind,
      })
      clearDraft()
      setIsNew(false)
      fetchNotes()
    }, t('project.saveFailed') + ': ')
    setSaving(false)
  }

  const startEdit = (note: Note) => {
    setEditingId(note.id)
    setEditDraft({
      content: note.content,
      title: note.title,
      tags: note.tags,
      kind: note.kind || 'other',
      pinned: note.pinned,
    })
    setIsNew(false)
  }

  const handleSaveEdit = async () => {
    if (editingId === null || !editDraft.content.trim()) return
    setSaving(true)
    await run(async () => {
      await updateNoteFull(
        editingId,
        editDraft.content.trim(),
        editDraft.title.trim(),
        editDraft.tags.trim(),
        editDraft.kind,
        !!editDraft.pinned
      )
      setEditingId(null)
      fetchNotes()
    }, t('project.saveFailed') + ': ')
    setSaving(false)
  }

  const handleMoveProject = async (noteId: number, targetProjectId: number) => {
    if (targetProjectId === projectId) return
    await run(async () => {
      await moveNote(noteId, targetProjectId)
      setNotes(prev => prev.filter(n => n.id !== noteId))
      if (editingId === noteId) setEditingId(null)
    }, t('project.saveFailed') + ': ')
  }

  const handleDelete = async (noteId: number) => {
    await run(async () => {
      await deleteNote(noteId)
      setNotes(prev => prev.filter(n => n.id !== noteId))
    }, t('project.saveFailed') + ': ')
  }

  const handlePin = async (note: Note) => {
    const nextPinned = !note.pinned
    setNotes(prev => prev.map(n => n.id === note.id ? { ...n, pinned: nextPinned } : n))
    // Roll back the optimistic toggle only when THIS pin actually failed —
    // run() resolves to success/failure (the old lastOpRef check fired on any
    // earlier failed operation and rolled back successful pins too).
    const ok = await run(async () => {
      await pinNote(note.id, nextPinned)
    }, t('project.saveFailed') + ': ')
    if (!ok) setNotes(prev => prev.map(n => n.id === note.id ? { ...n, pinned: note.pinned } : n))
  }

  if (loading) {
    return (
      <div className="panel-section">
        <h3>{t('knowledge.notesTitle', { defaultValue: '知识笔记' })}</h3>
        <div className="skeleton skeleton-text" style={{ height: 60, marginBottom: 8 }} />
        <div className="skeleton skeleton-text" style={{ height: 60 }} />
      </div>
    )
  }

  return (
    <div className="panel-section">
      <div className={s.header}>
        <h3>{t('knowledge.notesTitle', { defaultValue: '知识笔记' })} ({notes.length})</h3>
        {!isNew && editingId === null && (
          <button className="btn btn-sm btn-primary" onClick={startNew}>{t('project.createNote')}</button>
        )}
      </div>

      {error && (
        <ErrorBanner message={error} onRetry={retryLast} />
      )}

      <NoteFilterBar filter={filter} setFilter={setFilter} notesCount={notes.length} t={t} />

      {isNew && (
        <NoteEditor
          value={draft}
          onChange={setDraft}
          onSave={handleCreate}
          onCancel={() => { setIsNew(false); clearDraft() }}
          saving={saving}
        />
      )}

      {filteredNotes.length === 0 && !isNew ? (
        <p className="empty-hint">{filter !== 'all' ? t('knowledge.noMatch') : t('project.noNotesHint')}</p>
      ) : (
        <div className={s.list}>
          {filteredNotes.map(note => (
            <NoteCard
              key={note.id}
              note={note}
              editing={editingId === note.id}
              editDraft={editDraft}
              onEditDraftChange={setEditDraft}
              onSaveEdit={handleSaveEdit}
              onCancelEdit={() => setEditingId(null)}
              saving={saving}
              projects={projects}
              currentProjectId={projectId}
              onMoveProject={handleMoveProject}
              onPin={handlePin}
              onEdit={startEdit}
              onOpenHistory={vh.openVersionHistory}
              onDelete={handleDelete}
            />
          ))}
        </div>
      )}

      {vh.versionHistory !== null && (
        <VersionHistoryPanel
          versions={vh.versionHistory}
          restoringId={vh.restoringId}
          diffText={vh.diffText}
          onRestore={vh.handleRestoreVersion}
          onShowDiff={vh.handleShowDiff}
          onClose={vh.closeVersionHistory}
        />
      )}
    </div>
  )
}

/** NoteCard renders one note, switching to the inline editor when editing. */
function NoteCard({
  note, editing, editDraft, onEditDraftChange, onSaveEdit, onCancelEdit, saving,
  projects, currentProjectId, onMoveProject, onPin, onEdit, onOpenHistory, onDelete,
}: {
  note: Note
  editing: boolean
  editDraft: NoteDraft
  onEditDraftChange: (d: NoteDraft) => void
  onSaveEdit: () => void
  onCancelEdit: () => void
  saving: boolean
  projects: Project[]
  currentProjectId: number
  onMoveProject: (noteId: number, projectId: number) => void
  onPin: (note: Note) => void
  onEdit: (note: Note) => void
  onOpenHistory: (noteId: number) => void
  onDelete: (noteId: number) => void
}) {
  const { t } = useTranslation()
  const { armed, click } = useConfirmClick(() => { void onDelete(note.id) })

  if (editing) {
    return (
      <div className={s.card}>
        <NoteEditor
          value={editDraft}
          onChange={onEditDraftChange}
          onSave={onSaveEdit}
          onCancel={onCancelEdit}
          saving={saving}
          showPinned
          projects={projects}
          currentProjectId={currentProjectId}
          onMoveProject={(pid) => onMoveProject(note.id, pid)}
        />
      </div>
    )
  }

  return (
    <div className={`${s.card} ${note.pinned ? s.cardPinned : ''}`}>
      <div className={s.titleRow}>
        <span className={s.titleText}>{note.title || note.content.split('\n')[0] || t('project.noteWord')}</span>
        <div className={s.titleBadges}>
          {note.kind === 'knowledge' && <span className={s.badgeNoteSm}>{t('project.kinds.knowledge')}</span>}
          {note.source === 'claude' && <span className={`${s.badgeNoteSm} ${s.badgeSource}`}>Claude</span>}
          <button
            className={`pin-btn ${note.pinned ? 'pinned' : ''}`}
            onClick={() => onPin(note)}
            title={note.pinned ? t('project.unpinned') : t('project.pinned')}
          ><Icon name="pin" size={15} /></button>
        </div>
      </div>
      <div className={`${s.body} markdown-body`} dangerouslySetInnerHTML={{ __html: renderMarkdown(note.content) }} />
      {note.tags && (
        <div className={s.tagsRow}>
          {note.tags.split(',').map(tag => tag.trim()).filter(Boolean).map(tag => <span key={tag} className={s.tagChip}>#{tag}</span>)}
        </div>
      )}
      <div className={s.meta}>
        <span className={s.time}>
          {note.updated_at !== note.created_at
            ? `${t('project.updatedAtPrefix')} ${note.updated_at}`
            : `${t('project.createdAtPrefix')} ${note.created_at}`}
        </span>
        <div className={s.actions}>
          <button className="btn btn-sm" onClick={() => onEdit(note)}>{t('project.edit')}</button>
          <button
            className="btn btn-sm"
            onClick={() => onOpenHistory(note.id)}
            title={t('project.versionHistory')}
          >
            {t('project.history')}
          </button>
          <button
            className={`btn btn-sm ${armed ? 'btn-delete-confirm' : 'btn-danger'}`}
            onClick={click}
          >
            {armed ? t('project.confirmDelete') : t('common.delete')}
          </button>
        </div>
      </div>
    </div>
  )
}

export default NoteSection
