import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import Icon from '../../components/Icon'
import { getRepoCommits, type RepoCommit } from '../../api/client'

interface Props {
  repoId: number
  /** Shown in the header so the list is identifiable when scrolled into. */
  repoName: string
}

const LIMIT = 50

/**
 * One repository's commit log, inline under its row.
 *
 * Read from git on demand rather than the database: daily_stats stores
 * "who changed how many lines on which day", with no message and no SHA, so a
 * commit list cannot be assembled from what is persisted. That is also why
 * this works for a repo with no forge remote — nothing depends on a
 * reachable web URL.
 */
export default function RepoCommitList({ repoId, repoName }: Props) {
  const { t } = useTranslation()
  const [commits, setCommits] = useState<RepoCommit[] | null>(null)
  const [error, setError] = useState('')
  const [copiedHash, setCopiedHash] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setCommits(null)
    setError('')
    getRepoCommits(repoId, LIMIT)
      .then(list => { if (!cancelled) setCommits(list) })
      .catch((e: unknown) => {
        if (cancelled) return
        setError(e instanceof Error ? e.message : t('common.failed'))
        setCommits([])
      })
    return () => { cancelled = true }
  }, [repoId, t])

  // The SHA is what you paste into cherry-pick or a forge search, so it is
  // copyable — a commit row you cannot act on is just decoration.
  const copyHash = async (hash: string) => {
    try {
      await navigator.clipboard.writeText(hash)
      setCopiedHash(hash)
      setTimeout(() => setCopiedHash(null), 1500)
    } catch {
      /* clipboard unavailable; the hash is visible in the row regardless */
    }
  }

  return (
    <div className="repo-commits">
      {commits === null && <div className="repo-commits-status">{t('common.loading')}</div>}
      {error && <div className="repo-commits-status repo-commits-error">{error}</div>}
      {commits !== null && !error && commits.length === 0 && (
        <div className="repo-commits-status">{t('project.noCommits')}</div>
      )}
      {commits !== null && commits.length > 0 && (
        <>
          <div className="repo-commits-head">
            <span>{t('project.commitCount', { count: commits.length })}</span>
            {commits.length >= LIMIT && <span className="repo-commits-cap">{t('project.commitsCapped', { limit: LIMIT })}</span>}
          </div>
          <ul className="commit-feed">
            {commits.map((c) => (
              <li key={c.hash} className="commit-feed-item">
                <span className="commit-dot" />
                <div className="commit-feed-body">
                  <div className="commit-feed-msg" title={c.message}>{c.message}</div>
                  <div className="commit-feed-meta">
                    <span>{c.time}</span>
                    {c.branch && <span className="commit-branch">{c.branch}</span>}
                    <span className="commit-author">{c.author}</span>
                    <button
                      className={`commit-hash ${copiedHash === c.hash ? 'commit-hash-copied' : ''}`}
                      onClick={() => copyHash(c.hash)}
                      title={t('project.copyHash')}
                    >
                      <Icon name={copiedHash === c.hash ? 'check' : 'file-text'} size={10} />
                      {c.hash.slice(0, 7)}
                    </button>
                  </div>
                </div>
              </li>
            ))}
          </ul>
          <div className="repo-commits-repo">{repoName}</div>
        </>
      )}
    </div>
  )
}
