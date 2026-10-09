import { useState, useEffect, useRef, useMemo } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { getProjects, searchAll, SearchHit } from '../api/client'
import { useTranslation } from 'react-i18next'
import DOMPurify from 'dompurify'
import { useApiData } from '../hooks/useApiData'
import s from './CommandPalette.module.css'

interface Props {
  open: boolean
  onClose: () => void
}

// CommandPalette is a Cmd/Ctrl+K quick-switcher: search notes & todos across all
// projects and jump straight to a project. It surfaces the knowledge-search
// capability as a first-class keyboard action.
//
// Accessibility: implemented as a combobox-in-dialog (ARIA APG). The dialog traps
// focus, restores it to the trigger on close, and the input drives selection via
// aria-activedescendant over a role="listbox" of role="option" items.
export default function CommandPalette({ open, onClose }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [hits, setHits] = useState<SearchHit[]>([])
  // All-projects list for the quick-jump suggestions, shared with NoteSection
  // via the same cache key so only one request is ever made.
  const { data: projectsData } = useApiData(() => getProjects(undefined, false), [], { cacheKey: 'projects:all' })
  // Stabilize the fallback so the useMemo below doesn't re-run every render
  // (the `?? []` would otherwise allocate a fresh array on each pass).
  const projects = useMemo(() => projectsData ?? [], [projectsData])
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const previouslyFocused = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (open) {
      // Record the element that had focus (the trigger button) so we can restore it.
      previouslyFocused.current = document.activeElement as HTMLElement
      setQuery('') // eslint-disable-line react-hooks/set-state-in-effect
      setHits([])
      setActiveIndex(0)
      setTimeout(() => inputRef.current?.focus(), 30)
    } else {
      // Restore focus to the triggering element when the palette closes.
      const el = previouslyFocused.current
      if (el && typeof el.focus === 'function') {
        el.focus()
        previouslyFocused.current = null
      }
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    const trimmed = query.trim()
    if (!trimmed) { setHits([]); setActiveIndex(0); return } // eslint-disable-line react-hooks/set-state-in-effect
    let cancelled = false
    const timer = setTimeout(() => {
      searchAll(trimmed).then(r => {
        if (!cancelled) { setHits(r ?? []); setActiveIndex(0) }
      }).catch(() => { if (!cancelled) setHits([]) })
    }, 150)
    return () => { cancelled = true; clearTimeout(timer) }
  }, [query, open])

  const filteredProjects = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return projects.slice(0, 5)
    return projects.filter(p => p.name.toLowerCase().includes(q)).slice(0, 5)
  }, [projects, query])

  const totalItems = hits.length + filteredProjects.length
  const activeId = totalItems > 0 ? `cmdk-opt-${activeIndex}` : undefined

  // Keep the active option in view as the user navigates with the arrow keys,
  // so the visible highlight and aria-activedescendant stay in sync.
  useEffect(() => {
    if (!open || !activeId) return
    document.getElementById(activeId)?.scrollIntoView({ block: 'nearest' })
  }, [activeId, open])

  const goto = (index: number) => {
    if (index < hits.length) {
      const h = hits[index]
      if (h) { navigate(`/project/${h.project_id}`); onClose() }
    } else {
      const p = filteredProjects[index - hits.length]
      if (p) { navigate(`/project/${p.id}`); onClose() }
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setActiveIndex(i => Math.min(i + 1, totalItems - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActiveIndex(i => Math.max(i - 1, 0)) }
    else if (e.key === 'Enter') { e.preventDefault(); goto(activeIndex) }
    else if (e.key === 'Escape') { e.preventDefault(); onClose() }
    else if (e.key === 'Tab') {
      // Focus trap: the input is the only tabbable element inside the dialog,
      // so Tab / Shift+Tab cycles back to it instead of escaping to the page.
      e.preventDefault()
      inputRef.current?.focus()
    }
  }

  if (!open) return null

  return (
    <div className={s.cmdkOverlay} onClick={onClose} tabIndex={-1}>
      <div
        className={s.cmdk}
        role="dialog"
        aria-modal="true"
        aria-label={t('commandPalette.title')}
        onClick={e => e.stopPropagation()}
      >
        <input
          ref={inputRef}
          type="text"
          role="combobox"
          aria-label={t('commandPalette.searchLabel')}
          aria-expanded={totalItems > 0}
          aria-controls="cmdk-results"
          aria-autocomplete="list"
          aria-activedescendant={activeId}
          value={query}
          onChange={e => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={t('commandPalette.placeholder')}
          className={`cmdk-input ${s.cmdkInput}`}
        />
        <div className={s.cmdkResults} id="cmdk-results" role="listbox" aria-label={t('commandPalette.groupNotes')}>
          {hits.length === 0 && filteredProjects.length === 0 && (
            <div className={s.cmdkEmpty}>{query.trim() ? t('commandPalette.noResults', { defaultValue: 'No results' }) : t('commandPalette.startSearch', { defaultValue: 'Start searching' })}</div>
          )}
          {hits.length > 0 && <div className={s.cmdkGroup}>{t('commandPalette.groupNotes')}</div>}
          {hits.map((h, i) => (
            <Link
              key={`${h.type}-${h.id}`}
              id={`cmdk-opt-${i}`}
              to={`/project/${h.project_id}`}
              className={`${s.cmdkItem} ${activeIndex === i ? s.cmdkItemActive : ''}`}
              role="option"
              aria-selected={activeIndex === i}
              tabIndex={-1}
              onMouseEnter={() => setActiveIndex(i)}
              onClick={onClose}
            >
              <span className={`${s.cmdkType} ${h.type === 'note' ? s.cmdkTypeNote : s.cmdkTypeTodo}`}>{h.type === 'note' ? t('commandPalette.noteMark') : t('commandPalette.todoMark')}</span>
              <div className={s.cmdkItemBody}>
                <div className={s.cmdkItemTitle}>{h.title}</div>
                <div className={s.cmdkItemSub}>{h.project_name} · <span dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(h.snippet.slice(0, 60)) }} /></div>
              </div>
            </Link>
          ))}
          {filteredProjects.map((p, i) => {
            const idx = hits.length + i
            return (
              <Link
                key={p.id}
                id={`cmdk-opt-${idx}`}
                to={`/project/${p.id}`}
                className={`${s.cmdkItem} ${activeIndex === idx ? s.cmdkItemActive : ''}`}
                role="option"
                aria-selected={activeIndex === idx}
                tabIndex={-1}
                onMouseEnter={() => setActiveIndex(idx)}
                onClick={onClose}
              >
                <span className={`${s.cmdkType} ${s.cmdkTypeProject}`}>{t('commandPalette.projectMark')}</span>
                <div className={s.cmdkItemBody}>
                  <div className={s.cmdkItemTitle}>{p.name}</div>
                  <div className={s.cmdkItemSub}>{t('commandPalette.reposCount', { count: p.repo_count })}</div>
                </div>
              </Link>
            )
          })}
        </div>
        <div className={s.cmdkFoot}>
          <span>{t('commandPalette.footSelect')}</span><span>{t('commandPalette.footOpen')}</span><span>{t('commandPalette.footClose')}</span>
        </div>
        <div className="visually-hidden" role="status" aria-live="polite">
          {totalItems > 0 ? t('commandPalette.resultsCount', { count: totalItems }) : (query.trim() ? t('commandPalette.noResults', { defaultValue: 'No results' }) : '')}
        </div>
      </div>
    </div>
  )
}
