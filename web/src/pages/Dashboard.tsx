import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import SummaryBar from '../components/SummaryBar'
import GoalRing from '../components/GoalRing'
import Heatmap from '../components/Heatmap'
import StatusBar from '../components/StatusBar'
import DatePicker from '../components/DatePicker'
import ProjectCard from '../components/ProjectCard'
import ProjectSearchDropdown from './dashboard/ProjectSearchDropdown'
import ErrorBanner from '../components/ErrorBanner'
import Icon from '../components/Icon'
import { useDashboardData, type SortKey } from '../hooks/useDashboardData'
import s from './Dashboard.module.css'

function Dashboard() {
  const { t } = useTranslation()
  const {
    summary, dailyGoal, date, sortKey, confirmScan, showStarredOnly,
    displayedError, scanning, scanMsg, scanDoneMsg, loading,
    sorted, starredProjects, unstarredProjects,
    todoMap, noteMap, globalTodoCount, myAdded, isWorkday, sortOptions,
    setDate, setSortKey, setConfirmScan, setShowStarredOnly,
    handleScan, handleToggleStar, handleRefreshHistory, fetchSummary,
    refetchProjects, setError, setSummaryError,
  } = useDashboardData()

  return (
    <div className={s.page}>
      <h1 className="visually-hidden">{t('dashboard.title')}</h1>
      <div className="visually-hidden" role="status" aria-live="polite">
        {scanning ? (scanMsg || t('dashboard.scanning', { defaultValue: 'Scanning…' })) : scanDoneMsg}
      </div>
      <div className={s.fixed}>
        <div className={s.heroRow}>
          <div className={s.heroCard}>
            <GoalRing
              value={myAdded}
              goal={isWorkday ? dailyGoal : 0}
              label={isWorkday ? t('dashboard.todayGoal', { defaultValue: "Today's Goal" }) : t('dashboard.notWorkday', { defaultValue: 'Not a workday' })}
              sublabel={isWorkday ? `${myAdded} / ${dailyGoal} ${t('dashboard.linesUnit')}` : `${myAdded} ${t('dashboard.linesUnit')}`}
            />
            <div className={s.heroText}>
              <div className={s.heroEyebrow}>{date} · {isWorkday ? t('dashboard.workday', { defaultValue: 'Workday' }) : t('dashboard.weekendShort')}</div>
              <div className={s.heroTitle}>
                {isWorkday
                  ? (myAdded >= dailyGoal ? t('dashboard.goalReached', { defaultValue: "Today's goal reached" }) : t('dashboard.goalRemaining', { count: Math.max(dailyGoal - myAdded, 0) }))
                  : t('dashboard.weekend', { defaultValue: 'Happy weekend' })}
              </div>
              <div className={s.heroSub}>
                {t('dashboard.personalAdded')}{' '}
                <strong className={myAdded > 0 ? 'green' : 'muted-num'}>
                  {myAdded > 0 ? `+${myAdded}` : '0'}
                </strong>{' · '}
                {t('dashboard.filesShort')} <strong>{summary?.my_files || 0}</strong> ·
                {t('dashboard.reposInvolved', { count: summary?.repo_count || 0 })}
              </div>
            </div>
          </div>

          <SummaryBar summary={summary} globalTodoCount={globalTodoCount} />
        </div>

        <Heatmap onDayClick={setDate} />

        <div className={s.controls}>
          <DatePicker value={date} onChange={setDate} />
          <div className={s.actions}>
            <ProjectSearchDropdown onToggleStar={handleToggleStar} />
            <div className="filter-toggle">
              <button className={`filter-btn ${!showStarredOnly ? 'active' : ''}`} onClick={() => setShowStarredOnly(false)}>{t('dashboard.all', { defaultValue: 'All' })}</button>
              <button className={`filter-btn ${showStarredOnly ? 'active' : ''}`} onClick={() => setShowStarredOnly(true)}>{t('dashboard.starred', { defaultValue: 'Starred' })}</button>
            </div>
            <div className={s.sortControl}>
              <label htmlFor="dashboard-sort">{t('dashboard.sortBy')}</label>
              <select id="dashboard-sort" value={sortKey} onChange={(e) => setSortKey(e.target.value as SortKey)} className="form-input sort-select">
                {sortOptions.map(opt => <option key={opt.key} value={opt.key}>{opt.label}</option>)}
              </select>
            </div>
            {confirmScan ? (
              <div className="confirm-group">
                <span className="confirm-text">{t('dashboard.confirmRescan')}</span>
                <button className="btn btn-primary btn-sm" onClick={handleScan} disabled={scanning}>{t('common.confirm')}</button>
                <button className="btn btn-sm" onClick={() => setConfirmScan(false)}>{t('common.cancel')}</button>
              </div>
            ) : (
              <button className="btn btn-primary" onClick={() => setConfirmScan(true)} disabled={scanning}>
                {scanning ? (scanMsg || t('dashboard.scanning', { defaultValue: 'Processing...' })) : t('dashboard.rescan', { defaultValue: 'Rescan' })}
              </button>
            )}
          </div>
        </div>

        {displayedError && (
          <ErrorBanner message={displayedError} onRetry={() => { setError(''); setSummaryError(''); void fetchSummary(date); void refetchProjects() }} />
        )}
      </div>

      <div className={s.scroll}>
        {loading ? (
          <div className={s.projectGrid}>
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className={`${s.skeletonCard} skeleton-card`}>
                <div className={s.skeletonHeader}>
                  <div className="skeleton skeleton-text" style={{ width: '60%', height: 20 }} />
                </div>
                <div className={s.skeletonGrid}>
                  {Array.from({ length: 4 }).map((_, j) => (
                    <div key={j} className={s.skeletonStat}>
                      <div className="skeleton skeleton-text" style={{ width: 32, height: 10 }} />
                      <div className="skeleton skeleton-text" style={{ width: 40, height: 16 }} />
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        ) : sorted.length === 0 ? (
          <div className="empty-state">
            <div className="empty-icon">
              <Icon name={showStarredOnly ? 'star' : 'search'} size={32} filled={showStarredOnly} />
            </div>
            <h3>{showStarredOnly ? t('dashboard.starredOnly', { defaultValue: 'No starred projects' }) : t('dashboard.noProjects', { defaultValue: 'No project data' })}</h3>
            <p>
              {showStarredOnly
                ? t('dashboard.starMsg', { defaultValue: 'Star a project to follow it.' })
                : t('dashboard.scanMsg', { defaultValue: 'No repos found. Configure scan roots first.' })}
            </p>
            <div className="empty-actions">
              {showStarredOnly ? (
                // Clearing a filter is a secondary, reversible action — a filled
                // primary button overstates it and competes with the real
                // primary actions elsewhere on the page.
                <button className="btn btn-secondary" onClick={() => setShowStarredOnly(false)}>{t('dashboard.viewAllProjects')}</button>
              ) : (
                <>
                  <button className="btn btn-primary" onClick={() => setConfirmScan(true)}>{t('dashboard.startScan')}</button>
                  <Link to="/settings" className="btn btn-secondary">{t('dashboard.configureDirs')}</Link>
                </>
              )}
            </div>
          </div>
        ) : (
          <>
            {starredProjects.length > 0 && (
              <div className={s.section}>
                <div className={s.sectionHeader}>
                  <h2 className={s.sectionTitle}>{t('dashboard.starredSection')}</h2>
                  <span className={s.sectionCount}>{starredProjects.length}</span>
                </div>
                <div className={s.listFlat}>
                  {starredProjects.map(p => (
                    <ProjectCard
                      key={p.id}
                      project={p}
                      date={date}
                      todoCount={todoMap.get(p.id)}
                      noteCount={noteMap.get(p.id)}
                      dailyGoal={isWorkday ? dailyGoal : 0}
                      isWorkday={isWorkday}
                      onToggleStar={handleToggleStar}
                      onRefreshHistory={handleRefreshHistory}
                    />
                  ))}
                </div>
              </div>
            )}

            {unstarredProjects.length > 0 && (
              <div className={s.section}>
                <div className={s.sectionHeader}>
                  <h2 className={s.sectionTitle}>{t('dashboard.otherSection')}</h2>
                  <span className={s.sectionCount}>{unstarredProjects.length}</span>
                </div>
                <div className={`${s.projectGrid} ${s.projectGridMinimal}`}>
                  {unstarredProjects.map(p => (
                    <ProjectCard
                      key={p.id}
                      project={p}
                      date={date}
                      onToggleStar={handleToggleStar}
                    />
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </div>

      <StatusBar />
    </div>
  )
}

export default Dashboard
