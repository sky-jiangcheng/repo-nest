import { useTranslation } from 'react-i18next'
import { Summary } from '../api/client'
import s from './SummaryBar.module.css'

interface Props {
  summary: Summary | null
  globalTodoCount?: number
}

function SkeletonItem() {
  return (
    <div className={s.item}>
      <div className="skeleton skeleton-text" style={{width: 48}} />
      <div className="skeleton skeleton-value" style={{width: 36}} />
    </div>
  )
}

function SummaryBar({ summary, globalTodoCount }: Props) {
  const { t } = useTranslation()
  if (!summary) {
    return (
      <div className={s.bar}>
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
    <div className={s.bar}>
      <div className={s.item}>
        <span className={s.label}>{t('summaryBar.repos')}</span>
        <span className={s.value}>{summary.repo_count || 0}</span>
      </div>
      <div className={s.item}>
        <span className={s.label}>{t('summaryBar.teamAdded')}</span>
        <span className={added > 0 ? `${s.value} ${s.valueSuccess}` : `${s.value} ${s.valueZero}`}>
          {added > 0 ? '+' : ''}{added}
        </span>
      </div>
      <div className={s.item}>
        <span className={s.label}>{t('summaryBar.teamDeleted')}</span>
        {/* A deleted line count is always a magnitude; the minus sign only
            appears when there is something to subtract. Emitting it for zero
            produced a "-0" that read as a real (tiny) deletion. */}
        <span className={deleted > 0 ? `${s.value} ${s.valueDanger}` : `${s.value} ${s.valueZero}`}>
          {deleted > 0 ? `-${deleted}` : '0'}
        </span>
      </div>
      <div className={s.item}>
        <span className={s.label}>{t('summaryBar.personalAdded')}</span>
        <span className={mine > 0 ? `${s.value} ${s.valueSuccess}` : `${s.value} ${s.valueZero}`}>{mine}</span>
      </div>
      <div className={s.item}>
        <span className={s.label}>{t('summaryBar.personalFiles')}</span>
        <span className={s.value}>{summary.my_files || 0}</span>
      </div>
      <div className={s.item}>
        <span className={s.label}>{t('summaryBar.date')}</span>
        <span className={s.value}>{summary.is_workday ? t('common.workday') : t('common.weekend')}</span>
      </div>
      {globalTodoCount !== undefined && globalTodoCount > 0 && (
        <div className={s.item}>
          <span className={s.label}>{t('summaryBar.todos')}</span>
          <span className={`${s.value} ${s.valueTodo}`}>{globalTodoCount}</span>
        </div>
      )}
    </div>
  )
}

export default SummaryBar
