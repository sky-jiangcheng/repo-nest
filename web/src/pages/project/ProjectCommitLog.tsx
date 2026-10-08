import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import Icon from '../../components/Icon'
import { useToast } from '../../hooks/useToast'
import { getProjectCommits, getRepoCommits, type RepoCommit, type RepoInfo } from '../../api/client'

interface Props {
  projectId: number
  repos: RepoInfo[]
}

const LIMIT = 50

const shortPath = (p: string) => p.split('/').slice(-2).join('/')

/**
 * The commits tab's single log: one repo filter row over one timeline.
 *
 * Replaces the per-repo expander rows, where every repo was a closed drawer
 * and reading history meant opening them one at a time. "全部" merges all
 * repositories by time (the shape that answers "what happened lately"), and a
 * chip narrows to one repo's log with its totals and hand-off tools.
 *
 * Read from git on demand: daily_stats carries no message or SHA, so a commit
 * list cannot be assembled from the database, and this also works for a repo
 * with no forge remote.
 */
export default function ProjectCommitLog({ projectId, repos }: Props) {
  const { t } = useTranslation()
  const toast = useToast()
  // 'all' = merged cross-repo timeline; number = one repo's log. Tagged with the
  // project it was picked under: /project/:id reuses this instance across
  // projects, and a repo id selected in one project must not filter another's
  // log. Tagging beats the previous "reset in an effect", which also trips the
  // repo's react-hooks rule about synchronous setState inside an effect.
  const [picked, setPicked] = useState<{ projectId: number; repo: number | 'all' }>({ projectId: 0, repo: 'all' })
  const selected: number | 'all' = picked.projectId === projectId ? picked.repo : 'all'
  const choose = (repo: number | 'all') => setPicked({ projectId, repo })
  const [copiedHash, setCopiedHash] = useState<string | null>(null)
  const [copiedPath, setCopiedPath] = useState<string | null>(null)

  // A single-repo project has nothing to filter; show that repo's log and bar
  // directly rather than a one-chip filter row.
  const isSingle = repos.length === 1
  // Primitive, not a derived object — the fetch effect keys off it.
  const fetchRepoId = isSingle
    ? repos[0].id
    : typeof selected === 'number' ? selected : null

  // Result is stored per request key, so a response can never paint a view that
  // is no longer asking for it, and no reset is needed when the key changes —
  // that is what lets the effect below do nothing but fetch.
  const requestKey = `${projectId}|${fetchRepoId ?? 'all'}`
  const [result, setResult] = useState<{ key: string; commits: RepoCommit[]; error: string } | null>(null)
  const shown = result && result.key === requestKey ? result : null
  const commits = shown ? shown.commits : null
  const error = shown ? shown.error : ''

  useEffect(() => {
    let cancelled = false
    const key = requestKey
    const req = fetchRepoId === null
      ? getProjectCommits(projectId, LIMIT)
      : getRepoCommits(fetchRepoId, LIMIT)
    req
      .then(list => { if (!cancelled) setResult({ key, commits: list, error: '' }) })
      .catch((e: unknown) => {
        if (cancelled) return
        setResult({ key, commits: [], error: e instanceof Error ? e.message : t('common.failed') })
      })
    return () => { cancelled = true }
  }, [requestKey, fetchRepoId, projectId, t])

  const activeRepo = fetchRepoId === null ? null : repos.find(r => r.id === fetchRepoId) ?? null
  const repoTotals = activeRepo
    ? (activeRepo.stats || []).reduce(
        (acc, s) => ({ added: acc.added + s.lines_added, deleted: acc.deleted + s.lines_deleted }),
        { added: 0, deleted: 0 },
      )
    : null
  // In a merged list every row needs its repo named; in a filtered list the
  // label would repeat the active chip on every row.
  const showRepo = fetchRepoId === null

  const copyHash = async (hash: string) => {
    try {
      await navigator.clipboard.writeText(hash)
      setCopiedHash(hash)
      setTimeout(() => setCopiedHash(null), 1500)
    } catch {
      /* clipboard unavailable; the hash is visible in the row regardless */
    }
  }

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
    <div className="commit-log">
      {repos.length > 1 && (
        <div className="commit-filter" role="group" aria-label={t('project.subRepos')}>
          <button
            type="button"
            className={`commit-chip ${selected === 'all' ? 'commit-chip-active' : ''}`}
            aria-pressed={selected === 'all'}
            onClick={() => choose('all')}
          >
            {t('project.filterAll')}
          </button>
          {repos.map((repo) => (
            <button
              key={repo.id}
              type="button"
              className={`commit-chip ${selected === repo.id ? 'commit-chip-active' : ''}`}
              aria-pressed={selected === repo.id}
              title={repo.path}
              onClick={() => choose(repo.id)}
            >
              {shortPath(repo.path)}
            </button>
          ))}
        </div>
      )}

      {activeRepo && repoTotals && (
        <div className="commit-repo-bar">
          <span className="commit-repo-path" title={activeRepo.path}>{activeRepo.path}</span>
          <div className="repo-tools">
            <a
              className="repo-tool"
              href={`vscode://file/${activeRepo.path}`}
              title={t('project.openEditor')}
              aria-label={t('project.openEditor')}
            >
              <Icon name="zap" size={13} />
            </a>
            <button
              className={`repo-tool ${copiedPath === activeRepo.path ? 'repo-tool-done' : ''}`}
              onClick={() => copyPath(activeRepo.path)}
              title={t('project.copyPath')}
              aria-label={t('project.copyPath')}
            >
              <Icon name={copiedPath === activeRepo.path ? 'check' : 'file-text'} size={13} />
            </button>
            {activeRepo.web_url && (
              <a
                className="repo-tool"
                href={activeRepo.web_url}
                target="_blank"
                rel="noreferrer noopener"
                title={t('project.openForge')}
                aria-label={t('project.openForge')}
              >
                <Icon name="globe" size={13} />
              </a>
            )}
            <div className="repo-totals">
              {/* Sign only when non-zero: "+0"/"-0" on every idle repo turns
                  the bar into visual noise. */}
              <span className={repoTotals.added > 0 ? 'green' : 'muted-num'}>
                {repoTotals.added > 0 ? `+${repoTotals.added}` : '0'}
              </span>
              <span className={repoTotals.deleted > 0 ? 'red' : 'muted-num'}>
                {repoTotals.deleted > 0 ? `-${repoTotals.deleted}` : '0'}
              </span>
            </div>
          </div>
        </div>
      )}

      {commits === null && <div className="commit-log-status">{t('common.loading')}</div>}
      {error && <div className="commit-log-status commit-log-error">{error}</div>}
      {commits !== null && !error && commits.length === 0 && (
        <div className="commit-log-status">{t('project.noCommits')}</div>
      )}
      {commits !== null && commits.length > 0 && (
        <>
          <ul className="commit-feed">
            {commits.map((c, i) => {
              // In the merged view the repo label doubles as the way into
              // that repo's log — a click narrows the timeline instead of
              // sending the reader back up to the filter row.
              const repo = showRepo ? repos.find(r => r.path === c.repo) : undefined
              return (
                <li key={c.hash || `${c.time}-${i}`} className="commit-feed-item">
                  <span className="commit-dot" />
                  <div className="commit-feed-body">
                    <div className="commit-feed-msg" title={c.message}>{c.message}</div>
                    <div className="commit-feed-meta">
                      <span>{c.time}</span>
                      {showRepo && (repo ? (
                        <button
                          className="commit-repo commit-repo-btn"
                          title={`${c.repo} · ${t('project.filterRepoHint')}`}
                          onClick={() => choose(repo.id)}
                        >
                          {shortPath(c.repo)}
                        </button>
                      ) : (
                        <span className="commit-repo" title={c.repo}>{shortPath(c.repo)}</span>
                      ))}
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
              )
            })}
          </ul>
          <div className="commit-log-foot">
            <span>{t('project.commitCount', { count: commits.length })}</span>
            {commits.length >= LIMIT && (
              <span className="commit-log-cap">{t('project.commitsCapped', { limit: LIMIT })}</span>
            )}
          </div>
        </>
      )}
    </div>
  )
}
