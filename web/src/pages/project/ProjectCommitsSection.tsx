import { useTranslation } from 'react-i18next'
import type { ProjectOverview } from '../../api/client'

interface Props {
  overview: ProjectOverview
}

/**
 * The time-dimension summary of the Commits tab: aggregated activity metrics
 * and the recent commit feed. Split out of ProjectOverviewSection, where they
 * duplicated the heatmap/trends already shown in the same page's Commits tab.
 */
export default function ProjectCommitsSection({ overview }: Props) {
  const { t } = useTranslation()

  const hasActivity = !!overview.activity &&
    (overview.activity.total_commits > 0 || overview.activity.commit_rate_30d > 0)
  const hasFeed = (overview.recent_commits?.length ?? 0) > 0
  if (!hasActivity && !hasFeed) return null

  return (
    <div className="detail-section">
      <div className="section-header">
        <h2>{t('project.activity')}</h2>
      </div>

      {hasActivity && (
        <div className="overview-activity">
          <div className="activity-stats">
            <div className="activity-stat">
              <span className="activity-value">{overview.activity!.total_commits}</span>
              <span className="activity-label">{t('project.totalCommits')}</span>
            </div>
            <div className="activity-stat">
              <span className="activity-value">{overview.activity!.commit_rate_30d}</span>
              <span className="activity-label">{t('project.last30d')}</span>
            </div>
            <div className="activity-stat">
              <span className="activity-value">{overview.activity!.active_days}</span>
              <span className="activity-label">{t('project.activeDays90')}</span>
            </div>
            <div className="activity-stat">
              <span className="activity-value">{overview.activity!.active_months}</span>
              <span className="activity-label">{t('project.activeMonths')}</span>
            </div>
            {overview.activity!.last_commit_date && (
              <div className="activity-stat">
                <span className="activity-value-sm">{overview.activity!.last_commit_date}</span>
                <span className="activity-label">{t('project.lastCommit')}</span>
              </div>
            )}
          </div>
        </div>
      )}

      {hasFeed && (
        <div className="overview-commits">
          <h4 className="overview-sub-title">{t('project.recentCommits')}</h4>
          <ul className="commit-feed">
            {overview.recent_commits!.map((c, i) => (
              <li key={i} className="commit-feed-item">
                <span className="commit-dot" />
                <div className="commit-feed-body">
                  <div className="commit-feed-msg">{c.message}</div>
                  <div className="commit-feed-meta">
                    <span>{c.time}</span>
                    {c.branch && <span className="commit-branch">{c.branch}</span>}
                    <span className="commit-author">{c.author}</span>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
