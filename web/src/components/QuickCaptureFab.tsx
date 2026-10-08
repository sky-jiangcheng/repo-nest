import { useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Icon from './Icon'
import { createNoteWithMeta, createTodo, getProjects, type NoteKind, type Project } from '../api/client'
import AIAskPanel from './AIAskPanel'
import type { PushToast } from '../hooks/useToast'

type Tab = 'capture' | 'ask'

interface Props {
  onToast: PushToast
}

/**
 * One floating entry point for everything you can add to the knowledge base.
 *
 * It used to be quick-capture only, with AI Q&A living behind a separate
 * button in the project header — two floating/edge affordances for the same
 * "write something down about this project" intent, and the AI one only
 * reachable from a project page. Both are now tabs of the same panel, so the
 * project selector, the panel shell, and the save path are shared.
 *
 * Capture stays the default tab: it is the one that works with no
 * configuration and no network.
 */
export default function QuickCaptureFab({ onToast }: Props) {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState<Tab>('capture')
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
    if (open && tab === 'capture') textareaRef.current?.focus()
  }, [open, tab])

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

        <div className="fab-tabs" role="tablist" aria-label={t('fab.title')}>
          <button
            role="tab"
            aria-selected={tab === 'capture'}
            className={`fab-tab ${tab === 'capture' ? 'active' : ''}`}
            onClick={() => setTab('capture')}
          >
            <Icon name="plus" size={12} />
            {t('fab.tabCapture')}
          </button>
          <button
            role="tab"
            aria-selected={tab === 'ask'}
            className={`fab-tab ${tab === 'ask' ? 'active' : ''}`}
            onClick={() => setTab('ask')}
          >
            <Icon name="help" size={12} />
            {t('ai.title')}
          </button>
        </div>

        {projects.length === 0 ? (
          <p className="fab-empty">{t('fab.noProjects')}</p>
        ) : tab === 'capture' ? (
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
        ) : (
          <AIAskPanel
            projectId={projectId!}
            projectName={projects.find(p => p.id === projectId)?.name ?? ''}
            onToast={onToast}
            onGoToSettings={() => { close(); navigate('/settings?tab=ai') }}
          />
        )}
      </div>
    </>
  )
}
