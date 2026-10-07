import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Icon from './Icon'
import { createNoteWithMeta, createTodo, getProjects, type NoteKind, type Project } from '../api/client'

interface Props {
  onToast: (item: { kind: 'success' | 'error'; title: string; message?: string; duration?: number }) => void
}

/**
 * Floating quick-capture ball, mounted app-wide. One entry point for notes,
 * todos and quick thoughts: expand into a compact panel, pick the project
 * (defaults to the one you are already viewing), type, save. The panel stays
 * deliberately tiny — full editing lives on the project page.
 */
export default function QuickCaptureFab({ onToast }: Props) {
  const { t } = useTranslation()
  const location = useLocation()
  const [open, setOpen] = useState(false)
  const [projects, setProjects] = useState<Project[]>([])
  const [projectId, setProjectId] = useState<number | null>(null)
  const [content, setContent] = useState('')
  const [kind, setKind] = useState<NoteKind>('knowledge')
  const [saving, setSaving] = useState<'note' | 'todo' | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // Default the target project to whatever detail page the user is on.
  const routeProjectId = Number(location.pathname.match(/^\/project\/(\d+)/)?.[1] || 0)

  useEffect(() => {
    if (!open) return
    let cancelled = false
    getProjects().then(ps => {
      if (cancelled) return
      setProjects(ps)
      setProjectId(prev => prev ?? (routeProjectId || ps[0]?.id) ?? null)
    }).catch(() => { /* panel stays usable only when projects load; save disabled */ })
    return () => { cancelled = true }
  }, [open, routeProjectId])

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open])

  useEffect(() => {
    if (open) textareaRef.current?.focus()
  }, [open])

  const close = () => { setOpen(false); setContent(''); setSaving(null) }

  const firstLine = content.split('\n').map(l => l.trim()).find(l => l !== '') || ''

  const saveNote = async () => {
    if (!projectId || !content.trim() || saving) return
    setSaving('note')
    try {
      await createNoteWithMeta(projectId, content, { title: firstLine, kind, source: 'quick' })
      onToast({ kind: 'success', title: t('fab.noteSaved') })
      close()
    } catch (e) {
      onToast({ kind: 'error', title: t('fab.saveFailed'), message: e instanceof Error ? e.message : undefined })
      setSaving(null)
    }
  }

  const saveTodo = async () => {
    if (!projectId || !firstLine || saving) return
    setSaving('todo')
    try {
      await createTodo(projectId, firstLine)
      onToast({ kind: 'success', title: t('fab.todoSaved') })
      close()
    } catch (e) {
      onToast({ kind: 'error', title: t('fab.saveFailed'), message: e instanceof Error ? e.message : undefined })
      setSaving(null)
    }
  }

  if (!open) {
    return (
      <button
        className="fab-ball"
        onClick={() => setOpen(true)}
        aria-label={t('fab.open')}
        title={t('fab.open')}
      >
        <Icon name="plus" size={22} />
      </button>
    )
  }

  const canNote = projectId !== null && content.trim() !== '' && saving === null
  const canTodo = projectId !== null && firstLine !== '' && saving === null

  return (
    <>
      <div className="fab-backdrop" onClick={close} />
      <div className="fab-panel" role="dialog" aria-label={t('fab.open')}>
        <div className="fab-panel-head">
          <span className="fab-title">{t('fab.title')}</span>
          <button className="fab-close" onClick={close} aria-label={t('common.close', { defaultValue: 'Close' })}>
            <Icon name="close" size={14} />
          </button>
        </div>

        {projects.length === 0 ? (
          <p className="fab-empty">{t('fab.noProjects')}</p>
        ) : (
          <>
            <select
              className="fab-project form-input"
              value={projectId ?? ''}
              onChange={e => setProjectId(Number(e.target.value))}
              aria-label={t('fab.project')}
            >
              {projects.map(p => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>

            <textarea
              ref={textareaRef}
              className="fab-content form-input"
              rows={4}
              value={content}
              onChange={e => setContent(e.target.value)}
              placeholder={t('fab.placeholder')}
            />

            <div className="fab-kinds" role="radiogroup" aria-label={t('fab.kind')}>
              {(['knowledge', 'log', 'idea', 'other'] as NoteKind[]).map(k => (
                <button
                  key={k}
                  className={`filter-btn ${kind === k ? 'active' : ''}`}
                  onClick={() => setKind(k)}
                  role="radio"
                  aria-checked={kind === k}
                >
                  {t(`fab.kind_${k}`)}
                </button>
              ))}
            </div>

            <div className="fab-actions">
              <button className="btn btn-secondary btn-sm" onClick={saveTodo} disabled={!canTodo}>
                {saving === 'todo' ? t('fab.saving') : t('fab.saveTodo')}
              </button>
              <button className="btn btn-primary btn-sm" onClick={saveNote} disabled={!canNote}>
                {saving === 'note' ? t('fab.saving') : t('fab.saveNote')}
              </button>
            </div>
          </>
        )}
      </div>
    </>
  )
}
