import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  approveWikiPage, cancelCompileJob, deleteCompiledPage, getProjects,
  listCompileJobs, listPendingWikiPages, listRejectedWikiPages,
  rejectWikiPage, startCompileJob, startLintJob,
} from '../api/client'
import type { CompileJob, Project, WikiPendingPage } from '../api/client'
import ErrorBanner from '../components/ErrorBanner'
import Icon from '../components/Icon'
import { useToast } from '../hooks/useToast'
import s from './Review.module.css'

// Poll cadence for a running compile job. Deliberately slow: a note takes minutes,
// so a fast poll would hammer the backend for no user-visible gain. This is the
// only scheduling the page does — the work itself is a backend job.
const jobPollMs = 5000
const maxNotesPerRun = 10

function statusKey(status: string): string {
  // status_* keys exist for every job state; unknown states render verbatim so a
  // new backend status is visible rather than silently labelled "queued".
  return `review.status_${status}`
}

// kindLabel names which queue a row came from. A missing kind means the backend
// predates the column, and such a row is a compile job — so the fallback is the
// compile label rather than a blank badge.
function kindLabel(job: CompileJob, t: (k: string) => string): string {
  const kind = job.kind || 'compile'
  return kind === 'lint' ? t('review.kindLint') : t('review.kindCompile')
}

function ReviewPage() {
  const { t } = useTranslation()
  const toast = useToast()

  const [projects, setProjects] = useState<Project[]>([])
  const [projectFilter, setProjectFilter] = useState<number>(0) // 0 = all
  const [pending, setPending] = useState<WikiPendingPage[]>([])
  const [rejected, setRejected] = useState<WikiPendingPage[]>([])
  const [jobs, setJobs] = useState<CompileJob[]>([])
  const [loadError, setLoadError] = useState('')
  const [busy, setBusy] = useState(false)
  const [reloadToken, setReloadToken] = useState(0)
  const reload = useCallback(() => setReloadToken(n => n + 1), [])

  useEffect(() => {
    // House pattern: the effect body only wires promises and cleanup — every
    // setState happens inside a .then/.catch. Calling a setState-bearing function
    // directly here is what the react-hooks lint rule rejects, and a synchronous
    // setState on mount would cascade-render the whole page.
    let cancelled = false
    Promise.all([
      getProjects(),
      listPendingWikiPages(projectFilter),
      listRejectedWikiPages(projectFilter),
      listCompileJobs(projectFilter, '', 10),
    ]).then(([projs, pend, rej, jb]) => {
      if (cancelled) return
      setProjects(projs)
      setPending(pend)
      setRejected(rej)
      setJobs(jb)
      setLoadError('')
    }).catch(() => {
      if (!cancelled) setLoadError(t('review.loadError'))
    })
    return () => { cancelled = true }
  }, [projectFilter, reloadToken, t])

  // Poll only while a job is alive; stop the instant none are, so a finished
  // review session never keeps hammering the backend. The interval callback
  // setting state is fine — it is not the effect body.
  const anyActive = jobs.some(j => j.status === 'queued' || j.status === 'running')
  useEffect(() => {
    if (!anyActive) return
    const id = setInterval(reload, jobPollMs)
    return () => clearInterval(id)
  }, [anyActive, reload])

  const runGuarded = useCallback(async (fn: () => Promise<unknown>) => {
    setBusy(true)
    try {
      await fn()
      reload()
    } catch (e) {
      toast({ kind: 'error', title: t('review.actionError'), message: e instanceof Error ? e.message : undefined })
    } finally {
      setBusy(false)
    }
  }, [reload, toast, t])

  const onApprove = (id: number) => void runGuarded(() => approveWikiPage(id))
  const onReject = (id: number) => void runGuarded(() => rejectWikiPage(id))
  const onDelete = (id: number) => {
    if (!window.confirm(t('review.confirmDelete'))) return
    void runGuarded(() => deleteCompiledPage(id))
  }
  const onStart = () => void runGuarded(() => startCompileJob(projectFilter, maxNotesPerRun))
  // A queued lint job is per-project (db.CreateLintJob), so the button is
  // disabled on "all projects" rather than failing after the click.
  const onStartLint = () => void runGuarded(() => startLintJob(projectFilter))
  const onCancel = (id: number) => void runGuarded(() => cancelCompileJob(id))

  const projectLabel = useMemo(() => {
    const m = new Map<number, string>()
    for (const p of projects) m.set(p.id, p.name)
    return (pid: number) => (pid === 0 ? t('review.globalProject') : (m.get(pid) ?? t('review.globalProject')))
  }, [projects, t])

  return (
    <div className={s.page}>
      <header className={s.header}>
        <div>
          <h1 className={s.title}>{t('review.title')}</h1>
          <p className={s.subtitle}>{t('review.subtitle')}</p>
        </div>
        <div className={s.controls}>
          <label className={s.selectWrap}>
            <span className={s.selectLabel}>{t('review.projectFilter')}</span>
            <select value={projectFilter} onChange={e => setProjectFilter(Number(e.target.value))}>
              <option value={0}>{t('review.allProjects')}</option>
              {projects.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </label>
          <button className="btn btn-sm" onClick={reload}>{t('review.reload')}</button>
          <button
            className="btn btn-sm btn-primary"
            onClick={onStart}
            disabled={busy || projectFilter === 0}
            title={projectFilter === 0 ? t('review.projectRequired') : undefined}
          >
            {t('review.startCompile')}
          </button>
          {/* Not a primary button: two primaries on one toolbar compete, and
              this one is off by default anyway (wiki_lint_llm). */}
          <button
            className="btn btn-sm"
            onClick={onStartLint}
            disabled={busy || projectFilter === 0}
            title={projectFilter === 0 ? t('review.lintNeedsProject') : t('review.startLintHint')}
          >
            {t('review.startLint')}
          </button>
        </div>
      </header>

      {loadError && <ErrorBanner message={loadError} onRetry={reload} />}

      <section className={s.group}>
        <h2 className={s.groupTitle}>{t('review.jobHeading')}</h2>
        {jobs.length === 0
          ? <p className={s.empty}>{t('review.emptyJobs')}</p>
          : <ul className={s.jobList}>
            {jobs.map(j => (
              <li key={j.id} className={s.job}>
                <div className={s.jobTop}>
                  <span className={`${s.badge} ${s['badge_' + j.status] ?? ''}`}>{t(statusKey(j.status), j.status)}</span>
                  {/* One queue, two kernels (ADR-0016 待决 ①), so the counters are
                      labelled per kind: "pages created" is always 0 on a lint row
                      and "notes" is not what a lint row counts. */}
                  <span className={s.badge}>{kindLabel(j, t)}</span>
                  <span className={s.jobMeta}>
                    {j.kind === 'lint'
                      ? t('review.lintProgress', { done: j.notes_done, total: j.notes_total || j.requested_notes })
                      : t('review.progress', { done: j.notes_done, total: j.notes_total || j.requested_notes })}
                    {' · '}{j.kind === 'lint'
                      ? t('review.findingsCount', { n: j.findings })
                      : t('review.pagesCreated', { n: j.pages_created })}
                    {j.kind !== 'lint' && j.rejected_ops > 0 && <> · {t('review.rejectedOps', { n: j.rejected_ops })}</>}
                    {j.kind !== 'lint' && j.revision_todos > 0 && <> · {t('review.revisionTodos', { n: j.revision_todos })}</>}
                  </span>
                  {(j.status === 'queued' || j.status === 'running') && (
                    <button className="btn btn-sm" onClick={() => onCancel(j.id)} disabled={busy}>
                      {t('review.cancelJob')}
                    </button>
                  )}
                </div>
                {(j.status === 'running' || j.status === 'queued') && j.notes_total > 0 && (
                  <div className={s.progressTrack}>
                    <div className={s.progressFill}
                      style={{ width: `${Math.min(100, (j.notes_done / j.notes_total) * 100)}%` }} />
                  </div>
                )}
                {j.stopped && <p className={s.jobNote}>{t('review.stopped', { reason: j.stopped })}</p>}
                {j.error && <p className={s.jobError}>{t('review.error', { msg: j.error })}</p>}
              </li>
            ))}
          </ul>}
      </section>

      <section className={s.group}>
        <h2 className={s.groupTitle}>{t('review.pendingHeading')} <span className={s.count}>{pending.length}</span></h2>
        {pending.length === 0
          ? <p className={s.empty}>{t('review.emptyPending')}</p>
          : <ul className={s.cardList}>
            {pending.map(p => <ReviewCard key={p.page.id} item={p} projectLabel={projectLabel}
              busy={busy} onApprove={onApprove} onReject={onReject} />)}
          </ul>}
      </section>

      <section className={s.group}>
        <h2 className={s.groupTitle}>{t('review.rejectedHeading')} <span className={s.count}>{rejected.length}</span></h2>
        {rejected.length === 0
          ? <p className={s.empty}>{t('review.emptyRejected')}</p>
          : <ul className={s.cardList}>
            {rejected.map(p => <ReviewCard key={p.page.id} item={p} projectLabel={projectLabel}
              busy={busy} rejected onDelete={onDelete} />)}
          </ul>}
      </section>
    </div>
  )
}

function ReviewCard({
  item, projectLabel, busy, rejected = false, onApprove, onReject, onDelete,
}: {
  item: WikiPendingPage
  projectLabel: (pid: number) => string
  busy: boolean
  rejected?: boolean
  onApprove?: (id: number) => void
  onReject?: (id: number) => void
  onDelete?: (id: number) => void
}) {
  const { t } = useTranslation()
  const p = item.page
  return (
    <li className={`${s.card} ${rejected ? s.cardRejected : ''}`}>
      <div className={s.cardHead}>
        <span className={s.kind}>{p.kind}</span>
        <span className={s.slug}>{p.slug}</span>
        <span className={s.proj}>{projectLabel(p.project_id)}</span>
      </div>
      {rejected
        ? <p className={s.errLine}><Icon name="warning" size={14} /> {p.content}</p>
        : <pre className={s.body}>{p.content}</pre>}
      {item.source_note_ids?.length > 0 && (
        <p className={s.meta}>{t('review.sourceNotes')}: {item.source_note_ids.join(', ')}</p>
      )}
      {item.out_links?.length > 0 && (
        <p className={s.meta}>{t('review.outLinks')}: {item.out_links.map(l => l.slug).join(', ')}</p>
      )}
      <div className={s.actions}>
        {!rejected && (
          <>
            <button className="btn btn-sm btn-primary" disabled={busy} onClick={() => onApprove?.(p.id)}>
              <Icon name="check" size={13} /> {t('review.approve')}
            </button>
            <button className="btn btn-sm" disabled={busy} onClick={() => onReject?.(p.id)}>
              {t('review.reject')}
            </button>
          </>
        )}
        {rejected && (
          <button className="btn btn-sm btn-danger" disabled={busy} onClick={() => onDelete?.(p.id)}>
            {t('review.deletePermanently')}
          </button>
        )}
      </div>
    </li>
  )
}

export default ReviewPage
