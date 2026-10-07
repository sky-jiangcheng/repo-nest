import { useTranslation } from 'react-i18next'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import Icon from './Icon'
import type { Project } from '../api/client'

interface Props {
  project: Project
  date?: string
  todoCount?: number
  noteCount?: number
  dailyGoal?: number
  isWorkday?: boolean
  onToggleStar?: (id: number) => void
  onRefreshHistory?: (id: number) => Promise<void>
}

function ProjectCard({ project, date, todoCount, noteCount, dailyGoal = 0, isWorkday = true, onToggleStar, onRefreshHistory }: Props) {
  const { t } = useTranslation()
  const [refreshing, setRefreshing] = useState(false)
  const myAdded = project.my_added || 0
  const myDeleted = project.my_deleted || 0
  const to = date ? `/project/${project.id}?date=${date}` : `/project/${project.id}`

  const reachedGoal = isWorkday && myAdded > 0 && myAdded >= dailyGoal

  const handleStarClick = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    onToggleStar?.(project.id)
  }

  const handleRefreshClick = async (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (refreshing || !onRefreshHistory) return
    setRefreshing(true)
    try {
      await onRefreshHistory(project.id)
    } finally {
      setRefreshing(false)
    }
  }

  if (!project.is_starred) {
    return (
      <div className="project-card project-card-minimal">
        <button
          className="card-star"
          onClick={handleStarClick}
          title={t('project.star')}
          aria-label={t('project.star')}
        >
          <Icon name="star" size={16} />
        </button>
        {/* Unstarred repos only show name + star button (no detail page) — clicking
            would lead to an empty detail page with no stats, so it is non-interactive. */}
        <span className="card-name">{project.name}</span>
      </div>
    )
  }

  // The star and refresh buttons are siblings of the card Link (never nested
  // inside the anchor) so the markup stays valid and keyboard-friendly.
  // Starred rows are deliberately FLAT: a single line — name, today's number,
  // my +/-, repo count, badges — instead of the old tall hero card. One
  // starred repo used to render a full-width slab that read like a dropdown.
  return (
    <div className="project-card-shell project-card-shell-flat">
      <button
        className="card-star starred"
        onClick={handleStarClick}
        title={t('project.unstar')}
        aria-label={t('project.unstar')}
      >
        <Icon name="star" size={16} filled />
      </button>
      <Link to={to} className={`project-card project-card-flat ${reachedGoal ? 'card-goal-reached' : ''}`}>
        <span className="card-flat-name">{project.name}</span>
        <span className="card-flat-stats">
          <span className="flat-pair">
            <span className="flat-label">{t('project.todayAdded')}</span>
            <span className={`flat-num ${myAdded > 0 ? 'green' : 'muted-num'}`}>{myAdded > 0 ? `+${myAdded}` : '0'}</span>
          </span>
          <span className="flat-pair">
            <span className="flat-label">{t('project.added')}</span>
            <span className={`flat-num ${myAdded > 0 ? 'green' : 'muted-num'}`}>{myAdded > 0 ? `+${myAdded}` : '0'}</span>
          </span>
          <span className="flat-pair">
            <span className="flat-label">{t('project.deleted')}</span>
            <span className={`flat-num ${myDeleted > 0 ? 'red' : 'muted-num'}`}>{myDeleted > 0 ? `-${myDeleted}` : '0'}</span>
          </span>
          <span className="flat-pair">
            <span className="flat-label">{t('project.repo')}</span>
            <span className="flat-num">{project.repo_count || 0}</span>
          </span>
        </span>
        <span className="card-badges">
          {reachedGoal && <span className="badge badge-goal" title={t('project.goalReached', { defaultValue: '已达成今日目标' })}>{t('project.goalBadge')}</span>}
          {noteCount !== undefined && noteCount > 0 && (
            <span className="badge badge-note" title={t('project.noteBadgeTitle')}>{noteCount}</span>
          )}
          {todoCount !== undefined && todoCount > 0 && (
            <span className="badge badge-todo">{todoCount}</span>
          )}
          {project.below_standard && <span className="badge badge-warning">{t('project.belowBadge')}</span>}
        </span>
      </Link>
      <button
        className="card-refresh-btn"
        onClick={handleRefreshClick}
        disabled={refreshing}
        title={refreshing ? t('project.refreshingHistory', { defaultValue: 'Refreshing…' }) : t('project.refreshHistory', { defaultValue: 'Refresh history' })}
        aria-label={refreshing ? t('project.refreshingHistory', { defaultValue: 'Refreshing…' }) : t('project.refreshHistory', { defaultValue: 'Refresh history' })}
      >
        {refreshing ? (
          <Icon name="refresh-partial" size={14} className="spin" />
        ) : (
          <Icon name="refresh" size={14} />
        )}
      </button>
    </div>
  )
}

export default ProjectCard
