import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import Icon from '../../components/Icon'
import { useToast } from '../../hooks/useToast'
import type { RepoInfo } from '../../api/client'

interface Props {
  repos: RepoInfo[]
}

/**
 * Per-repository rows: totals, the two hand-off tools (editor deep link,
 * copy path), and a link to the repo's real commit history.
 *
 * It used to also list every per-author daily stat as a chip, behind a
 * "more" expander. That was removed deliberately — see the comment on
 * .repo-foot below.
 */
export default function RepoBreakdown({ repos }: Props) {
  const { t } = useTranslation()
  const toast = useToast()
  const [copiedPath, setCopiedPath] = useState<string | null>(null)

  // Two confirmations, not one: the icon flips to a check in place so the row
  // itself acknowledges the click, and a toast names what was copied. Swapping
  // only the title attribute (what this did before) is invisible — tooltips
  // render on hover, and the pointer is already on the button.
  const copyPath = async (path: string) => {
    try {
      await navigator.clipboard.writeText(path)
      setCopiedPath(path)
      toast({ kind: 'success', title: t('project.copied'), message: path })
      setTimeout(() => setCopiedPath(null), 1500)
    } catch {
      toast({ kind: 'error', title: t('project.copyFailed') })
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
        const authors = new Set((repo.stats || []).map((s) => s.author)).size
        return (
          <div key={repo.id} className="repo-item">
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
                  className={`repo-tool ${copiedPath === repo.path ? 'repo-tool-done' : ''}`}
                  onClick={() => copyPath(repo.path)}
                  title={t('project.copyPath')}
                  aria-label={t('project.copyPath')}
                >
                  <Icon name={copiedPath === repo.path ? 'check' : 'file-text'} size={13} />
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
            {/* Per-author daily line counts were a dead end: a wall of
                "all: +137 -124" chips the reader cannot act on and cannot
                reconcile with the repo total. What they actually want from a
                repo row is the commit history, so that is what the row
                offers — and when the repo has no browsable remote, the author
                count stays as a quiet fact rather than a broken link. */}
            <div className="repo-foot">
              {repo.web_url ? (
                <a
                  className="repo-history-link"
                  href={repo.web_url}
                  target="_blank"
                  rel="noreferrer noopener"
                >
                  <Icon name="globe" size={12} />
                  {t('project.viewCommits')}
                </a>
              ) : (
                <span className="repo-authors">
                  {t('project.authorCount', { count: authors })}
                </span>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}
