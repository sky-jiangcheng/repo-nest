import { useRef, useState } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import TrendChart from '../components/TrendChart'
import Heatmap from '../components/Heatmap'
import StatusBar from '../components/StatusBar'
import ScopeToggle from '../components/ScopeToggle'
import ProjectOverviewSection from './project/ProjectOverviewSection'
import ProjectCommitsSection from './project/ProjectCommitsSection'
import ErrorBanner from '../components/ErrorBanner'
import Icon from '../components/Icon'
import AIAskDialog from '../components/AIAskDialog'
import NoteSection from '../components/NoteSection'
import TodoSection from '../components/TodoSection'
import RepoBreakdown from './project/RepoBreakdown'
import { useProjectDetail } from '../hooks/useProjectDetail'
import { copyText } from '../utils/clipboard'

const TAB_KEYS = ['overview', 'commits', 'notes', 'todos'] as const
type DetailTab = typeof TAB_KEYS[number]

function ProjectDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const dateParam = searchParams.get('date') || ''

  const {
    project, overview, loading, error, setError,
    scope, setScope, trendData, totals, retry, handleLevelChange,
  } = useProjectDetail(id)

  // The page's sections live behind tabs instead of one long scroll: the
  // fixed header used to eat most of the viewport and every dynamic block
  // (heatmap, trends, repos, notes) sat below the fold. The active tab is
  // mirrored into ?tab= so a section is linkable; ?newNote=1 (Knowledge page
  // jump) lands on the notes tab where the create form actually lives.
  const initialTab = (): DetailTab => {
    if (searchParams.get('newNote') === '1') return 'notes'
    const raw = searchParams.get('tab')
    // 'repos' merged into 'commits': the per-repo breakdown IS the commit
    // view — heatmap/trends project-wide, breakdown by repository below.
    const normalized: DetailTab | null =
      raw === 'repos' ? 'commits'
      : raw && TAB_KEYS.includes(raw as DetailTab) ? (raw as DetailTab)
      : null
    return normalized ?? 'overview'
  }
  const [tab, setTab] = useState<DetailTab>(initialTab)

  const selectTab = (next: DetailTab) => {
    setTab(next)
    const params = new URLSearchParams(searchParams)
    params.set('tab', next)
    params.delete('newNote')
    setSearchParams(params, { replace: true })
  }

  const [copied, setCopied] = useState(false)
  const [askOpen, setAskOpen] = useState(false)
  const [actionMsg, setActionMsg] = useState('')
  const actionTimer = useRef<number | null>(null)

  const flashAction = (msg: string) => {
    setActionMsg(msg)
    if (actionTimer.current) clearTimeout(actionTimer.current)
    actionTimer.current = window.setTimeout(() => setActionMsg(''), 3500)
  }

  const onLevelChange = async (direction: 'up' | 'down') => {
    const r = await handleLevelChange(direction)
    if (r.ok) {
      flashAction(t('project.levelChanged'))
    } else if (r.error && r.error.toLowerCase().includes('not found')) {
      // MergeProjectUp/SplitProjectDown surface "no sibling here" as 404 —
      // translate it or the button reads as dead.
      flashAction(t('project.levelNoSibling'))
    } else {
      flashAction(r.error || t('common.failed'))
    }
  }

  // The shared context pack: one builder feeds both "Copy AI Context" and the
  // AI Q&A dialog's prompt pack.
  const buildContextLines = (): string[] | null => {
    if (!project) return null
    // This markdown feeds an AI model, not the UI: new section headers stay
    // in fixed English so the prompt-like output is stable across locales
    // (existing t() labels kept for continuity).
    const lines: string[] = [
      `# ${project.name}`,
      `path: ${project.root_path}`,
      `grouping: ${project.is_auto_grouped ? t('project.autoGroup') : t('project.manualGroup')}`,
    ]
    if (overview?.readme_excerpt) lines.push('', `## README\n${overview.readme_excerpt}`)
    if (overview?.tech_stack?.length) lines.push('', `## ${t('project.techStack')}\n${overview.tech_stack?.map(x => x.name).join(', ')}`)
    if (overview?.languages?.length) {
      lines.push('', `## Languages\n${overview.languages.slice(0, 8).map(l => `${l.language}: ${l.count}`).join('\n')}`)
    }
    if (overview?.activity && (overview.activity.total_commits > 0 || overview.activity.last_commit_date)) {
      lines.push(
        '',
        `## Activity\n- total commits: ${overview.activity.total_commits}\n- active days (90d): ${overview.activity.active_days}\n- commits (30d): ${overview.activity.commit_rate_30d}\n- last commit: ${overview.activity.last_commit_date}`,
      )
    }
    if (totals) {
      lines.push('', `## Stats (selected range)\n- added: ${totals.added}\n- deleted: ${totals.deleted}\n- files changed: ${totals.files}\n- active days: ${totals.active}`)
    }
    if (project.repos?.length) {
      const capped = project.repos.slice(0, 30)
      const more = project.repos.length - capped.length
      lines.push('', `## Repositories (${project.repos.length})\n${capped.map(r => `- ${r.path}`).join('\n')}${more > 0 ? `\n- ...and ${more} more` : ''}`)
    }
    if (overview?.recent_commits?.length) lines.push('', `## ${t('project.recentCommits')}\n${overview.recent_commits?.slice(0, 5).map(c => `- ${c.time} ${c.message}`).join('\n')}`)
    return lines
  }

  const handleCopyContext = async () => {
    const lines = buildContextLines()
    if (!lines) return
    try {
      await copyText(lines.join('\n'))
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      setError(t('common.failed'))
    }
  }

  const buildPrompt = (question: string): string => {
    const context = buildContextLines()?.join('\n') ?? ''
    return `${t('ai.promptHeader', { defaultValue: 'Question' })}: ${question}\n\n${context}`
  }

  if (loading) {
    return (
      <div className="project-detail">
        <div className="project-fixed">
          <div className="skeleton skeleton-text" style={{width: 200, height: 28, marginBottom: 8}} />
          <div className="skeleton skeleton-text" style={{width: '50%', height: 14, marginBottom: 20}} />
          <div className="skeleton skeleton-text" style={{width: '100%', height: 80, marginBottom: 16}} />
        </div>
        <div className="project-scroll">
          <div className="skeleton skeleton-text" style={{width: '100%', height: 280, marginBottom: 16}} />
        </div>
        <StatusBar />
      </div>
    )
  }

  if (error || !project) {
    // A missing project is not a transient failure: retrying the same request
    // will fail identically, so it gets a dead-end state with a way out rather
    // than a retry button. The backend message ("project not found") is
    // untranslated, so the not-found case is detected from it and re-rendered
    // in the UI language.
    const notFound = !error || /not found|不存在/i.test(error)
    if (notFound) {
      return (
        <div className="project-detail">
          <div className="empty-state">
            <div className="empty-icon"><Icon name="search" size={34} /></div>
            <h3>{t('project.notFoundTitle')}</h3>
            <p>{t('project.notFoundDesc')}</p>
            <div className="empty-actions">
              <button className="btn btn-primary" onClick={() => navigate('/dashboard')}>
                {t('project.backToDashboard')}
              </button>
            </div>
          </div>
          <StatusBar />
        </div>
      )
    }
    return (
      <div className="project-detail">
        <button className="btn btn-secondary back-btn" onClick={() => navigate('/dashboard')}>&larr; {t('project.backToDashboard')}</button>
        <ErrorBanner message={error || t('project.noRepos')} onRetry={retry} />
        <StatusBar />
      </div>
    )
  }

  return (
    <div className="project-detail">
      <div className="project-fixed">
        {actionMsg && <div className="message-banner" role="status">{actionMsg}</div>}

        {/* Compact head: identity + copy action on line 1, one stat strip on
            line 2, tab strip on line 3. Everything that used to be a stacked
            card here (title card, 4-column stats grid, meta row) collapsed
            into ~120px so the tab panels own the viewport. */}
        <div className="detail-head-compact">
          <div className="head-line1">
            <button className="btn btn-secondary btn-sm" onClick={() => navigate('/dashboard')}>
              &larr; {t('project.backToDashboard')}
            </button>
            <h1>{project.name}</h1>
            <span className="head-path" title={project.root_path}>{project.root_path}</span>
            <span className="level-control" title={t('project.groupLevelHint')}>
              <button className="btn btn-sm btn-icon" onClick={() => onLevelChange('down')} aria-label={t('project.levelDown')}><Icon name="minus" size={14} /></button>
              <span className="level-value">{t('project.groupLevel', { n: project.level_override || 0 })}</span>
              <button className="btn btn-sm btn-icon" onClick={() => onLevelChange('up')} aria-label={t('project.levelUp')}><Icon name="plus" size={14} /></button>
            </span>
            <button className="btn btn-secondary btn-sm" onClick={() => setAskOpen(true)}>
              <Icon name="help" size={13} /> {t('ai.title')}
            </button>
            <button className="btn btn-primary btn-sm" onClick={handleCopyContext}>
              {copied ? t('project.copied') : t('project.copyContext')}
            </button>
          </div>

          <div className="head-stats-line">
            <span className="head-stat"><span className="head-stat-label">{t('project.activeDays')}</span><span className="head-stat-value">{totals.active}</span></span>
            <span className="head-stat"><span className="head-stat-label">{t('project.fileChanges')}</span><span className="head-stat-value">{totals.files}</span></span>
            <span className="head-stat"><span className="head-stat-label">{t('project.added')}</span><span className={`head-stat-value ${totals.added > 0 ? 'green' : ''}`}>{totals.added > 0 ? `+${totals.added}` : '0'}</span></span>
            <span className="head-stat"><span className="head-stat-label">{t('project.deleted')}</span><span className={`head-stat-value ${totals.deleted > 0 ? 'red' : ''}`}>{totals.deleted > 0 ? `-${totals.deleted}` : '0'}</span></span>
            <span className="head-stat"><span className="head-stat-label">{t('project.subRepos')}</span><span className="head-stat-value">{project.repos?.length || 0}</span></span>
            {overview?.activity?.last_commit_date && (
              <span className="head-stat"><span className="head-stat-label">{t('project.lastCommit')}</span><span className="head-stat-value">{overview.activity.last_commit_date}</span></span>
            )}
            {overview?.languages?.[0] && (
              <span className="head-stat"><span className="head-stat-label">{t('project.mainLanguage')}</span><span className="head-stat-value">{overview.languages[0].language}</span></span>
            )}
            <span className="head-stat"><span className="head-stat-label">{t('project.grouping')}</span><span className="head-stat-value">{project.is_auto_grouped ? t('project.autoGroup') : t('project.manualGroup')}</span></span>
            {dateParam && (
              <span className="head-stat"><span className="head-stat-label">{t('project.datePill')}</span><span className="head-stat-value">{dateParam}</span></span>
            )}
          </div>

          <div className="detail-tabs" role="tablist" aria-label={t('project.sectionTabs', { defaultValue: 'Sections' })}>
            {TAB_KEYS.map((key) => (
              <button
                key={key}
                role="tab"
                id={`project-tab-${key}`}
                aria-selected={tab === key}
                aria-controls={`project-panel-${key}`}
                tabIndex={tab === key ? 0 : -1}
                className={`tab-btn ${tab === key ? 'tab-active' : ''}`}
                onClick={() => selectTab(key)}
                onKeyDown={(e) => {
                  const i = TAB_KEYS.indexOf(key)
                  if (e.key === 'ArrowRight') selectTab(TAB_KEYS[(i + 1) % TAB_KEYS.length])
                  else if (e.key === 'ArrowLeft') selectTab(TAB_KEYS[(i - 1 + TAB_KEYS.length) % TAB_KEYS.length])
                  else if (e.key === 'Home') selectTab(TAB_KEYS[0])
                  else if (e.key === 'End') selectTab(TAB_KEYS[TAB_KEYS.length - 1])
                  else return
                  e.preventDefault()
                }}
              >
                {t(`project.tab_${key}`)}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="project-scroll" role="tabpanel" id={`project-panel-${tab}`} aria-labelledby={`project-tab-${tab}`}>
          {tab === 'overview' && overview && <ProjectOverviewSection overview={overview} />}

          {tab === 'commits' && (
            <div className="commits-tab">
              {/* T-shape: the horizontal bar is the two bounded visuals sharing
                  one scope toggle; the stem below splits into variable-length
                  columns (repo breakdown grows with repo count, the commit
                  feed grows with history) instead of one long scroll. */}
              <div className="detail-section">
                <div className="section-header">
                  <h2>{t('heatmap.title')}</h2>
                  <ScopeToggle scope={scope} onChange={setScope} />
                </div>
                <div className="commit-visuals">
                  <div className="commit-visual commit-visual-heat">
                    <Heatmap projectId={Number(id)} scope={scope} onScopeChange={setScope} />
                  </div>
                  <div className="commit-visual commit-visual-trend">
                    <h4 className="overview-sub-title">{t('project.trendTitle')}</h4>
                    {trendData.labels.length > 0 ? (
                      <TrendChart labels={trendData.labels} datasets={trendData.datasets} />
                    ) : (
                      <div className="empty-section">
                        {t('project.noDataInRange')}
                        {scope !== 'all' && (
                          <div className="empty-actions">
                            {scope === 'week' && (
                              <button className="btn btn-secondary btn-sm" onClick={() => setScope('month')}>
                                {t('heatmap.show30d')}
                              </button>
                            )}
                            <button className="btn btn-secondary btn-sm" onClick={() => setScope('all')}>
                              {t('heatmap.showAll')}
                            </button>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              </div>

              <div className="commits-columns">
                <div className="detail-section">
                  <div className="section-header">
                    <h2>{t('project.subRepos')} ({project.repos?.length || 0})</h2>
                  </div>
                  <RepoBreakdown repos={project.repos || []} />
                </div>

                {overview && <ProjectCommitsSection overview={overview} />}
              </div>
            </div>
          )}

          {tab === 'notes' && (
            /* key remounts the section when the project changes: /project/:id
                reuses this page instance, and without the key the previous
                project's open draft was persisted into the new project's
                localStorage key — and could be saved there. */
            <NoteSection key={`notes-${id}`} projectId={Number(id)} autoNew={searchParams.get('newNote') === '1'} />
          )}

          {tab === 'todos' && (
            <TodoSection key={`todos-${id}`} projectId={Number(id)} />
          )}
      </div>

      <StatusBar />
      {askOpen && project && (
        <AIAskDialog
          projectId={project.id}
          onClose={() => setAskOpen(false)}
          buildPrompt={buildPrompt}
          onToast={(item) => flashAction(item.kind === 'success' ? item.title : `${item.title}${item.message ? ' ' + item.message : ''}`)}
        />
      )}
    </div>
  )
}

export default ProjectDetailPage
