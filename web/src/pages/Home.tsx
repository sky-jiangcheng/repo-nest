import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import DOMPurify from 'dompurify'
import Dashboard from './Dashboard'
import { listRecentHandoffs, searchAll, type HandoffEntry, type SearchHit } from '../api/client'
import { useDebouncedCallback } from '../hooks/useDebouncedCallback'
import s from './Home.module.css'

// Home is the D25 first screen (ADR-0017): the memory-ring entry — recent
// session handoffs plus global search — side by side with the project status
// overview. The right column embeds the intact Dashboard component: per the
// ADR the dashboard, goal ring, heatmap and workday alert stay on the first
// screen, unchanged and visible by default.
export default function Home() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [entries, setEntries] = useState<HandoffEntry[] | null>(null)
  const [query, setQuery] = useState('')
  const [hits, setHits] = useState<SearchHit[]>([])
  const [searched, setSearched] = useState(false)

  useEffect(() => {
    let cancelled = false
    listRecentHandoffs(10)
      .then(r => { if (!cancelled) setEntries(r ?? []) })
      .catch(() => { if (!cancelled) setEntries([]) })
    return () => { cancelled = true }
  }, [])

  const runSearch = useDebouncedCallback((q: string) => {
    const trimmed = q.trim()
    if (!trimmed) { setHits([]); setSearched(false); return }
    searchAll(trimmed)
      .then(r => { setHits(r ?? []); setSearched(true) })
      .catch(() => { setHits([]); setSearched(true) })
  }, 150)

  const onQueryChange = (value: string) => {
    setQuery(value)
    runSearch(value)
  }

  return (
    <div className={s.grid}>
      <section className={s.ring} aria-label={t('home.ringTitle', { defaultValue: '记忆环' })}>
        <h1 className="visually-hidden">{t('home.title', { defaultValue: '首页' })}</h1>
        <div className={s.ringHead}>
          <h2>{t('home.ringTitle', { defaultValue: '记忆环' })}</h2>
          <p className={s.ringSub}>{t('home.ringSub', { defaultValue: '最近的会话交接与全局检索' })}</p>
        </div>

        <div className={s.searchBlock}>
          <input
            type="search"
            className="search-input"
            value={query}
            onChange={e => onQueryChange(e.target.value)}
            placeholder={t('home.searchPlaceholder', { defaultValue: '搜索笔记与待办…' })}
            aria-label={t('home.searchLabel', { defaultValue: '全局检索' })}
          />
          {searched && (
            <div className={s.hits} role="list" aria-label={t('home.resultsLabel', { defaultValue: '搜索结果' })}>
              {hits.length === 0 && (
                <div className={s.noHits}>{t('home.noResults', { defaultValue: '没有匹配的结果' })}</div>
              )}
              {hits.map(h => (
                <button
                  type="button"
                  key={`${h.type}-${h.id}`}
                  role="listitem"
                  className={s.hit}
                  onClick={() => navigate(`/project/${h.project_id}`)}
                >
                  <span className={`hit-type-${h.type}`}>
                    {h.type === 'note'
                      ? t('commandPalette.noteMark', { defaultValue: '笔记' })
                      : t('commandPalette.todoMark', { defaultValue: '待办' })}
                  </span>
                  <span className={s.hitBody}>
                    <span className={s.hitTitle}>{h.title}</span>
                    <span className={s.hitSub}>
                      {h.project_name} · <span dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(h.snippet.slice(0, 80)) }} />
                    </span>
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>

        <div className={s.handoffBlock}>
          <h3>{t('home.recentHandoffs', { defaultValue: '最近 handoff' })}</h3>
          {entries === null ? null : entries.length === 0 ? (
            <p className="empty-hint">{t('home.noHandoffs', { defaultValue: '还没有 handoff——会话结束或手动捕捉后会出现在这里' })}</p>
          ) : (
            <ul className={s.handoffList}>
              {entries.map(e => (
                <li key={e.note_id}>
                  <button
                    type="button"
                    className={s.handoffItem}
                    onClick={() => navigate(`/project/${e.project_id}`)}
                    title={t('home.openProject', { defaultValue: '打开项目 {{name}}', name: e.project_name })}
                  >
                    <span className={s.handoffTitle}>{e.title}</span>
                    <span className={s.handoffMeta}>
                      <span className={s.handoffProject}>{e.project_name}</span>
                      {e.auto_captured && <span className={s.autoBadge}>{t('home.autoCaptured', { defaultValue: '自动' })}</span>}
                      <span>{e.updated_at.slice(0, 16)}</span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>

      <section className={s.dashWrap} aria-label={t('home.projectStatus', { defaultValue: '项目状态一览' })}>
        <Dashboard />
      </section>
    </div>
  )
}
