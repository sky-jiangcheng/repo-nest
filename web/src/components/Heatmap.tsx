import { toDateStr } from '../utils/dates'
import { useTranslation } from 'react-i18next'
import { useEffect, useState, useMemo } from 'react'
import { getHeatmapData, type HeatmapDay } from '../api/client'
import type { Scope } from './ScopeToggle'

const DAYS_PER_WEEK = 7

const SCOPE_DAYS: Record<Scope, number> = {
  week: 7,
  month: 30,
  all: 364,
}

interface Props {
  onDayClick?: (date: string) => void
  /** Restrict the heatmap to one project's repositories (0 = global). */
  projectId?: number
  /** Time window shown: week = 7d, month = 30d, all = ~52w. */
  scope?: Scope
  /** Provided on pages whose scope is user-facing: lets the empty state offer
      one-click widening instead of a dead "no activity" message. */
  onScopeChange?: (s: Scope) => void
}

function getLevel(day: HeatmapDay | null): number {
  if (!day) return 0
  const total = (day.lines_added || 0) + (day.lines_deleted || 0)
  if (total === 0) return 0
  if (total < 100) return 1
  if (total < 300) return 2
  if (total < 600) return 3
  return 4
}

/** Local calendar date as YYYY-MM-DD (never UTC, so cells match the user's day). */
function generateGrid(days: HeatmapDay[], daysToShow: number) {
  const dayMap = new Map<string, HeatmapDay>()
  for (const d of days) {
    // Defensive: the API contract is YYYY-MM-DD, but rows written before the
    // backend truncated the datetime leaked "2025-10-11T00:00:00Z" here and
    // every grid cell missed its lookup.
    dayMap.set(d.date.slice(0, 10), d)
  }

  // Anchor the window end at today and align the start to the beginning of a week.
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const start = new Date(today)
  start.setDate(start.getDate() - (daysToShow - 1))
  const startDay = start.getDay()
  start.setDate(start.getDate() - startDay)

  const weeks = Math.ceil((daysToShow + startDay) / 7)
  const grid: (HeatmapDay | null)[][] = []

  for (let w = 0; w < weeks; w++) {
    const week: (HeatmapDay | null)[] = []
    for (let d = 0; d < DAYS_PER_WEEK; d++) {
      const date = new Date(start)
      date.setDate(date.getDate() + w * 7 + d)
      const key = toDateStr(date)
      week.push(dayMap.get(key) || null)
    }
    grid.push(week)
  }

  return grid
}

export default function Heatmap({ onDayClick, projectId = 0, scope = 'all', onScopeChange }: Props) {
  const { t } = useTranslation()
  const [days, setDays] = useState<HeatmapDay[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    getHeatmapData(projectId)
      .then(res => { if (!cancelled) setDays(res.days) })
      .catch(() => { if (!cancelled) setDays([]) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [projectId])

  const grid = useMemo(() => generateGrid(days, SCOPE_DAYS[scope]), [days, scope])
  const stats = useMemo(() => {
    // Stats reflect the visible window only. Dates are YYYY-MM-DD, so a
    // lexicographic range check is exact and avoids per-day Date parsing.
    const today = new Date()
    today.setHours(0, 0, 0, 0)
    const cutoff = new Date(today)
    cutoff.setDate(cutoff.getDate() - (SCOPE_DAYS[scope] - 1))
    const todayStr = toDateStr(today)
    const cutoffStr = toDateStr(cutoff)
    const visible = days.filter(d => d.date >= cutoffStr && d.date <= todayStr)
    return {
      active: visible.filter(d => (d.lines_added || 0) + (d.lines_deleted || 0) > 0).length,
      commits: visible.reduce((sum, d) => sum + (d.commits || 0), 0),
      added: visible.reduce((sum, d) => sum + (d.lines_added || 0), 0),
      deleted: visible.reduce((sum, d) => sum + (d.lines_deleted || 0), 0),
    }
  }, [days, scope])

  const renderCell = (day: HeatmapDay | null, key: number) => {
    const clickable = day && onDayClick
    return (
      <div
        key={key}
        className={`heatmap-cell-simple level-${getLevel(day)}`}
        role={clickable ? 'button' : undefined}
        tabIndex={clickable ? 0 : undefined}
        aria-label={day ? `${day.date}: +${day.lines_added} -${day.lines_deleted}` : t('heatmap.noData')}
        title={day ? `${day.date}: +${day.lines_added} -${day.lines_deleted}` : ''}
        onClick={clickable ? () => onDayClick!(day!.date) : undefined}
        onKeyDown={clickable ? (e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onDayClick!(day!.date)
          }
        } : undefined}
      />
    )
  }

  if (loading) {
    return (
      <div className="heatmap-simple">
        <div className="heatmap-loading">{t('common.loading', { defaultValue: '加载中…' })}</div>
      </div>
    )
  }

  if (stats.active === 0) {
    // The component holds a full year of days regardless of scope, so it can
    // tell "nothing anywhere" from "nothing in this narrow window" — the
    // latter gets one-click widening instead of a dead end.
    const today = new Date()
    today.setHours(0, 0, 0, 0)
    const todayStr = toDateStr(today)
    const mk = (n: number) => {
      const c = new Date(today)
      c.setDate(c.getDate() - (n - 1))
      return toDateStr(c)
    }
    const activeSince = (n: number) =>
      days.some(d => d.date >= mk(n) && d.date <= todayStr && (d.lines_added || 0) + (d.lines_deleted || 0) > 0)
    const canWiden = onScopeChange && scope !== 'all' && activeSince(364)
    return (
      <div className="heatmap-simple heatmap-empty-state">
        <div className="heatmap-empty-body">
          <p className="empty-hint">{t('heatmap.noActivityInRange')}</p>
          {canWiden && (
            <div className="empty-actions">
              {scope === 'week' && activeSince(30) && (
                <button className="btn btn-secondary btn-sm" onClick={() => onScopeChange('month')}>
                  {t('heatmap.show30d')}
                </button>
              )}
              <button className="btn btn-secondary btn-sm" onClick={() => onScopeChange('all')}>
                {t('heatmap.showAll')}
              </button>
            </div>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="heatmap-simple">
      <div className="heatmap-header">
        <div className="heatmap-stats">
          <div className="heatmap-stat">
            <span className="heatmap-stat-label">{t('heatmap.active')}</span>
            <span className="heatmap-stat-value">{stats.active}</span>
          </div>
          <div className="heatmap-stat">
            <span className="heatmap-stat-label">{t('heatmap.commits')}</span>
            <span className="heatmap-stat-value">{stats.commits}</span>
          </div>
          <div className="heatmap-stat">
            <span className="heatmap-stat-label">{t('heatmap.added')}</span>
            <span className="heatmap-stat-value">{stats.added}</span>
          </div>
          <div className="heatmap-stat">
            <span className="heatmap-stat-label">{t('heatmap.deleted')}</span>
            <span className="heatmap-stat-value">-{stats.deleted}</span>
          </div>
        </div>
      </div>

      {/* One row of week-columns sharing the card width. Columns flex to fill
          when the card is wide enough; below ~53×14px the row scrolls
          horizontally instead of squeezing cells into slivers. */}
      <div className="heatmap-grid-simple heatmap-grid-year" role="grid" aria-label={t('heatmap.title')}>
        {grid.map((week, wi) => (
          <div key={wi} className="heatmap-week-simple" role="gridcolumn">
            {week.map((day, di) => renderCell(day, di))}
          </div>
        ))}
      </div>

      <div className="heatmap-legend-simple">
        <span>{t('heatmap.less')}</span>
        <div className="heatmap-cell-simple level-0" />
        <div className="heatmap-cell-simple level-1" />
        <div className="heatmap-cell-simple level-2" />
        <div className="heatmap-cell-simple level-3" />
        <div className="heatmap-cell-simple level-4" />
        <span>{t('heatmap.more')}</span>
      </div>
    </div>
  )
}
