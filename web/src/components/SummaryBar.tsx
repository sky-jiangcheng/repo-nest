import { useTranslation } from 'react-i18next'
import { Summary } from '../api/client'

interface Props {
  summary: Summary | null
  globalTodoCount?: number
}

function SkeletonItem() {
  return (
    <div className="summary-item">
      <div className="skeleton skeleton-text" style={{width: 48}} />
      <div className="skeleton skeleton-value" style={{width: 36}} />
    </div>
  )
}

function SummaryBar({ summary, globalTodoCount }: Props) {
  const { t } = useTranslation()
  if (!summary) {
    return (
      <div className="summary-bar">
        <SkeletonItem />
        <SkeletonItem />
        <SkeletonItem />
        <SkeletonItem />
        <SkeletonItem />
        <SkeletonItem />
      </div>
    )
  }

  // A signed zero ("-0" / "+0") is noise: it implies a movement that did not
  // happen. Only draw the sign when there is something to sign, and keep the
  // muted neutral colour for zero so the eye only catches real movement.
  const added = summary.total_added || 0
  const deleted = summary.total_deleted || 0
  const mine = summary.my_added || 0

  return (
    <div className="summary-bar">
      <div className="summary-item">
        <span className="summary-label">{t('summaryBar.repos')}</span>
        <span className="summary-value">{summary.repo_count || 0}</span>
      </div>
      <div className="summary-item">
        <span className="summary-label">{t('summaryBar.teamAdded')}</span>
        <span className={added > 0 ? 'summary-value green' : 'summary-value zero'}>
          {added > 0 ? '+' : ''}{added}
        </span>
      </div>
      <div className="summary-item">
        <span className="summary-label">{t('summaryBar.teamDeleted')}</span>
        {/* A deleted line count is always a magnitude; the minus sign only
            appears when there is something to subtract. Emitting it for zero
            produced a "-0" that read as a real (tiny) deletion. */}
        <span className={deleted > 0 ? 'summary-value red' : 'summary-value zero'}>
          {deleted > 0 ? `-${deleted}` : '0'}
        </span>
      </div>
      <div className="summary-item">
        <span className="summary-label">{t('summaryBar.personalAdded')}</span>
        <span className={mine > 0 ? 'summary-value green' : 'summary-value zero'}>{mine}</span>
      </div>
      <div className="summary-item">
        <span className="summary-label">{t('summaryBar.personalFiles')}</span>
        <span className="summary-value">{summary.my_files || 0}</span>
      </div>
      <div className="summary-item">
        <span className="summary-label">{t('summaryBar.date')}</span>
        <span className="summary-value">{summary.is_workday ? t('common.workday') : t('common.weekend')}</span>
      </div>
      {globalTodoCount !== undefined && globalTodoCount > 0 && (
        <div className="summary-item">
          <span className="summary-label">{t('summaryBar.todos')}</span>
          <span className="summary-value todo">{globalTodoCount}</span>
        </div>
      )}
    </div>
  )
}

export default SummaryBar
