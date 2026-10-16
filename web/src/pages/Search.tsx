import { useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import DOMPurify from 'dompurify'
import Icon from '../components/Icon'
import { getProjects, listAllTags, searchAll, type Project, type SearchHit } from '../api/client'
import { useApiData } from '../hooks/useApiData'
import s from './Search.module.css'

// Search is the top-level global search page (ADR-0024). It replaces the
// inline search box the old home memory ring carried: one big input, results
// grouped by 项目 / 笔记 / 待办, and the query mirrored into ?q= so
// recommendations and deep links stay addressable.
//
// Strings live in the `search` namespace (locales/*/search.json), referenced
// with the `search:` prefix from the default common namespace. The idle
// (no-query) state is the recommendation surface: hot projects ranked
// client-side by recent personal additions (Project.my_added — GetProjects
// defaults that to yesterday, a stable "recent" anchor), with starred/repo
// count tiebreakers, and hot tags from ListAllTags that fill the input on
// click. All of it reuses existing endpoints — no backend change.
export default function Search() {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  // ?q= is the single deep-link entry; afterwards state owns the input and
  // mirrors back to the URL (replace — typing must not spam history).
  const [query, setQuery] = useState(() => searchParams.get('q') ?? '')
  const [hits, setHits] = useState<SearchHit[]>([])
  const [searched, setSearched] = useState(false)

  // Shared caches: the projects list is the same one the command palette and
  // dashboard use; tags join it so the recommendation row costs one request.
  const { data: projectsData } = useApiData(() => getProjects(undefined, false), [], { cacheKey: 'projects:all' })
  const { data: tagsData } = useApiData(() => listAllTags(), [], { cacheKey: 'tags:all' })

  // Debounced search + URL mirroring, both keyed on the query. setSearchParams
  // is referentially stable in react-router, so the sync cannot loop.
  useEffect(() => {
    const q = query.trim()
    setSearchParams(q ? { q } : {}, { replace: true })
    if (!q) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setHits([]); setSearched(false)
      return
    }
    let cancelled = false
    const timer = setTimeout(() => {
      searchAll(q)
        .then(r => { if (!cancelled) { setHits(r ?? []); setSearched(true) } })
        .catch(() => { if (!cancelled) { setHits([]); setSearched(true) } })
    }, 150)
    return () => { cancelled = true; clearTimeout(timer) }
  }, [query, setSearchParams])

  const noteHits = useMemo(() => hits.filter(h => h.type === 'note'), [hits])
  const todoHits = useMemo(() => hits.filter(h => h.type === 'todo'), [hits])

  // Project-name matches are client-side over the shared projects list —
  // the same fallback the command palette offers.
  const projectHits = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return [] as Project[]
    return (projectsData ?? []).filter(p => p.name.toLowerCase().includes(q)).slice(0, 8)
  }, [projectsData, query])

  const hotProjects = useMemo(() => {
    return [...(projectsData ?? [])]
      .sort((a, b) =>
        b.my_added - a.my_added ||
        Number(b.is_starred) - Number(a.is_starred) ||
        b.repo_count - a.repo_count ||
        a.name.localeCompare(b.name))
      .slice(0, 6)
  }, [projectsData])

  const hotTags = useMemo(() => (tagsData ?? []).slice(0, 12), [tagsData])

  const noResults = searched && noteHits.length === 0 && todoHits.length === 0 && projectHits.length === 0

  return (
    <div className={s.page}>
      <h1 className={s.title}>{t('search:title', { defaultValue: '全局搜索' })}</h1>

      <input
        type="search"
        className={`search-input ${s.bigInput}`}
        value={query}
        onChange={e => setQuery(e.target.value)}
        placeholder={t('search:placeholder', { defaultValue: '搜索笔记、待办与项目…' })}
        aria-label={t('search:title', { defaultValue: '全局搜索' })}
        autoFocus
      />

      {searched ? (
        <div className={s.results} aria-label={t('search:results', { defaultValue: '搜索结果' })}>
          {projectHits.length > 0 && (
            <section className={s.group} aria-label={t('search:groupProjects', { defaultValue: '项目' })}>
              <h2 className={s.groupTitle}>
                {t('search:groupProjects', { defaultValue: '项目' })}
                <span className={s.groupCount}>{projectHits.length}</span>
              </h2>
              <ul className={s.rowList}>
                {projectHits.map(p => (
                  <li key={p.id}>
                    <Link to={`/project/${p.id}`} className={s.row}>
                      <span className={`${s.typeBadge} ${s.typeProject}`}>{t('commandPalette.projectMark', { defaultValue: '项目' })}</span>
                      <span className={s.rowBody}>
                        <span className={s.rowTitle}>{p.name}</span>
                        <span className={s.rowSub}>
                          {t('commandPalette.reposCount', { count: p.repo_count })}
                          {' · '}+{p.my_added}
                        </span>
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          )}

          {noteHits.length > 0 && (
            <section className={s.group} aria-label={t('search:groupNotes', { defaultValue: '笔记' })}>
              <h2 className={s.groupTitle}>
                {t('search:groupNotes', { defaultValue: '笔记' })}
                <span className={s.groupCount}>{noteHits.length}</span>
              </h2>
              <ul className={s.rowList}>
                {noteHits.map(h => <HitRow key={`note-${h.id}`} hit={h} t={t} />)}
              </ul>
            </section>
          )}

          {todoHits.length > 0 && (
            <section className={s.group} aria-label={t('search:groupTodos', { defaultValue: '待办' })}>
              <h2 className={s.groupTitle}>
                {t('search:groupTodos', { defaultValue: '待办' })}
                <span className={s.groupCount}>{todoHits.length}</span>
              </h2>
              <ul className={s.rowList}>
                {todoHits.map(h => <HitRow key={`todo-${h.id}`} hit={h} t={t} />)}
              </ul>
            </section>
          )}

          {noResults && (
            <div className={s.noResults}>
              <p>{t('search:noResults', { defaultValue: '没有匹配的结果' })}</p>
              {hotTags.length > 0 && <p className={s.noResultsHint}>{t('search:tryHotTags', { defaultValue: '试试从热门标签开始' })}</p>}
            </div>
          )}

          <div className="visually-hidden" role="status" aria-live="polite">
            {noResults ? t('search:noResults', { defaultValue: '没有匹配的结果' }) : t('search:resultsCount', { count: hits.length + projectHits.length, defaultValue: '{{count}} 条结果' })}
          </div>
        </div>
      ) : (
        <div className={s.reco}>
          <section className={s.group} aria-label={t('search:hotProjects', { defaultValue: '热门项目' })}>
            <h2 className={s.groupTitle}>
              {t('search:hotProjects', { defaultValue: '热门项目' })}
              <span className={s.groupHint}>{t('search:hotProjectsSub', { defaultValue: '按最近个人新增排序' })}</span>
            </h2>
            <ul className={s.hotGrid}>
              {hotProjects.map(p => (
                <li key={p.id}>
                  <Link to={`/project/${p.id}`} className={s.hotCard}>
                    <span className={s.hotName}>
                      {p.is_starred && <Icon name="star" size={12} filled />}
                      {p.name}
                    </span>
                    <span className={s.hotSub}>
                      {t('commandPalette.reposCount', { count: p.repo_count })}
                      {' · '}
                      {t('search:recentAdded', { count: p.my_added, defaultValue: '最近 +{{count}}' })}
                    </span>
                  </Link>
                </li>
              ))}
              {hotProjects.length === 0 && (
                <li className={s.recoEmpty}>{t('search:noHotProjects', { defaultValue: '还没有项目数据——先在设置里配置扫描目录' })}</li>
              )}
            </ul>
          </section>

          <section className={s.group} aria-label={t('search:hotTags', { defaultValue: '热门标签' })}>
            <h2 className={s.groupTitle}>
              {t('search:hotTags', { defaultValue: '热门标签' })}
              <span className={s.groupHint}>{t('search:hotTagsHint', { defaultValue: '点击标签立即检索' })}</span>
            </h2>
            <div className={s.tagRow}>
              {hotTags.map(tag => (
                <button type="button" key={tag} className={s.tagChip} onClick={() => setQuery(tag)}>
                  {tag}
                </button>
              ))}
              {hotTags.length === 0 && (
                <span className={s.recoEmpty}>{t('search:noHotTags', { defaultValue: '笔记里还没有标签' })}</span>
              )}
            </div>
          </section>
        </div>
      )}
    </div>
  )
}

// One search hit row — shared between the notes and todos groups. The snippet
// comes back from FTS5 with <mark> highlights; DOMPurify keeps the same
// sanitization contract as the command palette and the old home ring.
function HitRow({ hit, t }: { hit: SearchHit; t: TFunction }) {
  return (
    <li>
      <Link to={`/project/${hit.project_id}`} className={s.row}>
        <span className={`hit-type-${hit.type} ${s.typeBadge}`}>
          {hit.type === 'note'
            ? t('commandPalette.noteMark', { defaultValue: '笔记' })
            : t('commandPalette.todoMark', { defaultValue: '待办' })}
        </span>
        <span className={s.rowBody}>
          <span className={s.rowTitle}>{hit.title}</span>
          <span className={s.rowSub}>
            {hit.project_name} · <span dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(hit.snippet.slice(0, 80)) }} />
          </span>
        </span>
      </Link>
    </li>
  )
}
