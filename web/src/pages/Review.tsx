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
import { renderMarkdown } from '../utils/markdown'
import s from './Review.module.css'

// Poll cadence for a running compile job. Deliberately slow: a note takes minutes,
// so a fast poll would hammer the backend for no user-visible gain. This is the
// only scheduling the page does — the work itself is a backend job.
const jobPollMs = 5000
const maxNotesPerRun = 10

// Job rows render {done} / {total} with total = notes_total || requested_notes.
// A queued job legitimately has both at 0 only when the worker has not claimed
// it yet AND nothing was requested — which cannot happen (StartCompileJob
// always requests ≥1 note). The other 0/0 shape is real data: the worker was
// killed before it could snapshot the note list. Rendering "0 / 0" there reads
// like a bug report, so those rows show only the status badge until real
// numbers exist.
function jobProgressText(j: CompileJob, t: (k: string, v?: Record<string, unknown>) => string): string {
  const total = j.notes_total || j.requested_notes
  if (total <= 0) return ''
  return j.kind === 'lint'
    ? t('review.lintProgress', { done: j.notes_done, total })
    : t('review.progress', { done: j.notes_done, total })
}

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
    }).catch((e) => {
      // Surface the underlying message next to the generic banner. A bare
      // "加载审核队列失败" once hid the real cause ("unknown method:
      // ListPendingWikiPages" — a stale reponest-server binary predating the
      // review bindings) for two days; the RPC layer rejects with either an
      // Error or a plain string (Wails mode), so handle both shapes.
      if (cancelled) return
      const detail = e instanceof Error ? e.message : String(e)
      setLoadError(detail ? `${t('review.loadError')}: ${detail}` : t('review.loadError'))
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
      {/* The nav bar already names this tab ("审核"), so a visible title would
          be duplication — it stays for screen readers and the document outline
          (same call as the knowledge page). */}
      <h1 className="visually-hidden">{t('review.title')}</h1>

      <div className={s.toolbar}>
        <select
          className={s.select}
          value={projectFilter}
          onChange={e => setProjectFilter(Number(e.target.value))}
          aria-label={t('review.projectFilter')}
        >
          <option value={0}>{t('review.allProjects')}</option>
          {projects.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
        <div className={s.toolbarActions}>
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
      </div>

      <div className={s.content}>
        <p className={s.lead}>{t('review.subtitle')}</p>

        {loadError && <ErrorBanner message={loadError} onRetry={reload} />}

        {/* Jobs and rejected sections only exist when they have content: a
            reviewer who just wants the queue should not scroll past two empty
            boxes on every visit. */}
        {jobs.length > 0 && (
          <section>
            <div className={s.sectionHead}>
              <h2>{t('review.jobHeading')}</h2>
            </div>
            <ul className={s.jobList}>
              {jobs.map(j => (
                <JobRow key={j.id} job={j} projectLabel={projectLabel} busy={busy} onCancel={onCancel} />
              ))}
            </ul>
          </section>
        )}

        <section>
          <div className={s.sectionHead}>
            <h2>{t('review.pendingHeading')}</h2>
            <span className={s.countChip}>{pending.length}</span>
          </div>
          {pending.length === 0
            ? <div className="empty-state small">
                <div className="empty-icon"><Icon name="file-text" size={32} /></div>
                <h3>{t('review.emptyPendingTitle')}</h3>
                <p>{t('review.emptyPendingMsg')}</p>
              </div>
            : <ul className={s.cardList}>
                {pending.map(p => (
                  <ReviewCard key={p.page.id} item={p} projectLabel={projectLabel}
                    busy={busy} onApprove={onApprove} onReject={onReject} />
                ))}
              </ul>}
        </section>

        {rejected.length > 0 && (
          <section>
            <div className={s.sectionHead}>
              <h2>{t('review.rejectedHeading')}</h2>
              <span className={s.countChip}>{rejected.length}</span>
            </div>
            <ul className={s.cardList}>
              {rejected.map(p => (
                <ReviewCard key={p.page.id} item={p} projectLabel={projectLabel}
                  busy={busy} rejected onDelete={onDelete} />
              ))}
            </ul>
          </section>
        )}
      </div>
    </div>
  )
}

function JobRow({
  job, projectLabel, busy, onCancel,
}: {
  job: CompileJob
  projectLabel: (pid: number) => string
  busy: boolean
  onCancel: (id: number) => void
}) {
  const { t } = useTranslation()
  const active = job.status === 'queued' || job.status === 'running'
  return (
    <li className={s.job}>
      <div className={s.jobTop}>
        <span className={`${s.status} ${s['status_' + job.status] ?? ''}`}>
          {job.status === 'running' && <span className={s.dot} />}
          {t(statusKey(job.status), job.status)}
        </span>
        <span className={s.jobKind}>{kindLabel(job, t)}</span>
        <span className={s.jobProject}>{projectLabel(job.project_id)}</span>
        <span className={s.jobProgress}>{jobProgressText(job, t)}</span>
        {active && (
          <button className={`btn btn-sm ${s.cancel}`} onClick={() => onCancel(job.id)} disabled={busy}>
            {t('review.cancelJob')}
          </button>
        )}
      </div>
      {active && job.notes_total > 0 && (
        <div className={s.progressTrack}>
          <div className={s.progressFill}
            style={{ width: `${Math.min(100, (job.notes_done / job.notes_total) * 100)}%` }} />
        </div>
      )}
      {/* One queue, two kernels (ADR-0016 待决 ①), so the counters are
          labelled per kind: "pages created" is always 0 on a lint row and
          "notes" is not what a lint row counts. Chips are only emitted for
          non-zero stats — a wall of "新建 0 页" chips is noise, and the
          done>0 disjunct keeps an in-flight row's first chip visible from
          note one. */}
      <div className={s.jobStats}>
        {job.kind === 'lint'
          ? (job.findings > 0 || job.notes_done > 0) && (
              <span className={s.stat}>{t('review.findingsCount', { n: job.findings })}</span>
            )
          : <>
              {(job.pages_created > 0 || job.notes_done > 0) && (
                <span className={s.stat}>{t('review.pagesCreated', { n: job.pages_created })}</span>
              )}
              {job.pages_updated > 0 && <span className={s.stat}>{t('review.pagesUpdated', { n: job.pages_updated })}</span>}
              {job.links_created > 0 && <span className={s.stat}>{t('review.linksCreated', { n: job.links_created })}</span>}
              {job.rejected_ops > 0 && <span className={s.stat}>{t('review.rejectedOps', { n: job.rejected_ops })}</span>}
              {job.revision_todos > 0 && <span className={s.stat}>{t('review.revisionTodos', { n: job.revision_todos })}</span>}
            </>}
      </div>
      {job.stopped && <p className={s.jobNote}>{t('review.stopped', { reason: job.stopped })}</p>}
      {job.error && <p className={s.jobError}>{t('review.error', { msg: job.error })}</p>}
    </li>
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
        <span className={`${s.kindBadge} ${s['kindBadge_' + p.kind] ?? ''}`}>
          {t(`review.kind_${p.kind}`, p.kind)}
        </span>
        {/* Title first — the slug is the machine key, the title is what a
            reviewer compares against the sources. A missing title renders the
            slug instead so a row never opens nameless. */}
        <span className={s.cardTitle}>{p.title || p.slug}</span>
        <span className={s.cardProject}>{projectLabel(p.project_id)}</span>
      </div>
      <p className={s.slug}>{p.slug}</p>
      {/* Render the body as markdown (the same pipeline the knowledge base
          uses): compiler output IS markdown — headings, lists, [[wikilinks]] —
          and a raw <pre> made every pending page read like a dump. The
          rejected branch stays plain text on purpose: its content is an error
          placeholder, not a document. renderMarkdown strips frontmatter and
          sanitizes (DOMPurify), matching NoteSection. */}
      {rejected
        ? <p className={s.errLine}><Icon name="warning" size={14} /> {p.content}</p>
        : <div className={`${s.body} markdown-body`} dangerouslySetInnerHTML={{ __html: renderMarkdown(p.content) }} />}
      {item.source_note_ids?.length > 0 && (
        <div className={s.metaRow}>
          <span className={s.metaLabel}>{t('review.sourceNotes')}</span>
          {item.source_note_ids.map(id => (
            <span key={id} className={s.chip}>{t('review.noteRef', { n: id })}</span>
          ))}
        </div>
      )}
      {item.in_links?.length > 0 && (
        <div className={s.metaRow}>
          <span className={s.metaLabel}>{t('review.inLinks')}</span>
          {item.in_links.map(l => (
            <span key={`${l.page_id}-${l.relation}`} className={s.chip}>{l.title || l.slug}</span>
          ))}
        </div>
      )}
      {item.out_links?.length > 0 && (
        <div className={s.metaRow}>
          <span className={s.metaLabel}>{t('review.outLinks')}</span>
          {item.out_links.map(l => (
            <span key={`${l.page_id}-${l.relation}`} className={s.chip}>{l.title || l.slug}</span>
          ))}
        </div>
      )}
      {/* Secondary action left, primary right — the page's whole job is the
          approve/decline decision, so the pair reads as one dialog. */}
      <div className={s.actions}>
        {!rejected && (
          <>
            <button className="btn btn-sm" disabled={busy} onClick={() => onReject?.(p.id)}>
              {t('review.reject')}
            </button>
            <button className="btn btn-sm btn-primary" disabled={busy} onClick={() => onApprove?.(p.id)}>
              <Icon name="check" size={13} /> {t('review.approve')}
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
