import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import Icon from '../../components/Icon'
import type { RepoInfo } from '../../api/client'

interface Props {
  repos: RepoInfo[]
}

const COLLAPSED_TAGS = 5

/**
 * Per-repository commit breakdown. A repo row is enterable: "more" expands
 * the full per-author list inline, and each row hands off to a real editor
 * via the vscode:// deep link (RepoNest is a read-only knowledge base — code
 * editing belongs to the user's editor, this just closes the distance).
 */
export default function RepoBreakdown({ repos }: Props) {
  const { t } = useTranslation()
  const [expandedId, setExpandedId] = useState<number | null>(null)
  const [copiedPath, setCopiedPath] = useState<string | null>(null)

  const copyPath = async (path: string) => {
    try {
      await navigator.clipboard.writeText(path)
      setCopiedPath(path)
      setTimeout(() => setCopiedPath(null), 1500)
    } catch {
      /* clipboard unavailable — the path is also shown in the tooltip */
    }
  }

  return (
    <div className="repo-list">
      {repos.map((repo) => {
        const repoTotals = (repo.stats || []).reduce(
          (acc, s) => ({
            added: acc.added + s.lines_added,
            deleted: acc.deleted + s.lines_deleted,
            files: acc.files + s.files_changed,
          }),
          { added: 0, deleted: 0, files: 0 }
        )
        const stats = repo.stats || []
        const expanded = expandedId === repo.id
        const visibleTags = expanded ? stats : stats.slice(0, COLLAPSED_TAGS)
        return (
          <div key={repo.id} className={`repo-item ${expanded ? 'repo-item-expanded' : ''}`}>
            <div className="repo-header">
              <div className="repo-path" title={repo.path}>{repo.path.split('/').slice(-2).join('/')}</div>
              <div className="repo-tools">
                <a
                  className="repo-tool"
                  href={`vscode://file/${repo.path}`}
                  title={t('project.openEditor')}
                  aria-label={t('project.openEditor')}
                >
                  <Icon name="zap" size={13} />
                </a>
                <button
                  className="repo-tool"
                  onClick={() => copyPath(repo.path)}
                  title={copiedPath === repo.path ? t('project.copied') : t('project.copyPath')}
                  aria-label={t('project.copyPath')}
                >
                  <Icon name="file-text" size={13} />
                </button>
                <div className="repo-totals">
                  {/* Sign only when non-zero: "+0"/"-0" on every idle repo
                      turns the list into visual noise. */}
                  <span className={repoTotals.added > 0 ? 'green' : 'muted-num'}>
                    {repoTotals.added > 0 ? `+${repoTotals.added}` : '0'}
                  </span>
                  <span className={repoTotals.deleted > 0 ? 'red' : 'muted-num'}>
                    {repoTotals.deleted > 0 ? `-${repoTotals.deleted}` : '0'}
                  </span>
                </div>
              </div>
            </div>
            {stats.length > 0 && (
              <div className="repo-stats">
                {visibleTags.map((stat) => (
                  <span key={stat.id} className="stat-tag" title={`${stat.stat_date} · ${stat.author}`}>
                    {stat.author}:{' '}
                    <span className={stat.lines_added > 0 ? 'green' : 'muted-num'}>
                      {stat.lines_added > 0 ? `+${stat.lines_added}` : '0'}
                    </span>{' '}
                    <span className={stat.lines_deleted > 0 ? 'red' : 'muted-num'}>
                      {stat.lines_deleted > 0 ? `-${stat.lines_deleted}` : '0'}
                    </span>
                  </span>
                ))}
                {stats.length > COLLAPSED_TAGS && (
                  <button
                    className="stat-tag more repo-more-btn"
                    onClick={() => setExpandedId(expanded ? null : repo.id)}
                  >
                    {expanded
                      ? t('project.showLess')
                      : t('project.moreCount', { count: stats.length - COLLAPSED_TAGS })}
                  </button>
                )}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
