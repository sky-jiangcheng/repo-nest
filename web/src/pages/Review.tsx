import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  approveWikiPage, cancelCompileJob, deleteCompiledPage, diffWikiPageVersion, findMergeCandidates,
  getProjects, listCompileJobs, listPendingWikiPages, listRejectedWikiPages, listWikiPageVersions,
  mergeWikiPages, rejectWikiPage, runCommunitySummary, startCompileJob, startLintJob,
} from '../api/client'
import type {
  CompileJob, MergeCandidate, MergeCandidateReport, Project, WikiCommunityReport,
  WikiPageVersion, WikiPendingPage,
} from '../api/client'
import DiffViewer from '../components/DiffViewer'
import ErrorBanner from '../components/ErrorBanner'
import Icon from '../components/Icon'
import { useConfirmClick } from '../hooks/useConfirmClick'
import { useToast } from '../hooks/useToast'
import { renderMarkdown } from '../utils/markdown'
import s from './Review.module.css'

// Poll cadence for a running compile job. Deliberately slow: a note takes minutes,
// so a fast poll would hammer the backend for no user-visible gain. This is the
// only scheduling the page does — the work itself is a backend job.
const jobPollMs = 5000
const maxNotesPerRun = 10
// How many merge pairs the review surface asks for. The backend's own default
// (mergeDefaultTopK) is the same number; passing it explicitly keeps the note
// ("仅显示前 N 对") true no matter what the binding's default becomes.
const mergeTopK = 20

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

// A dismissed pair is keyed by project too: "ignore" means "not in this view",
// and switching project is a different view. Nothing is persisted — the moment
// the page unmounts the pair is suggested again, which is the point (a local
// dismissal must never become a silent global "these are not duplicates").
const mergePairKey = (projectId: number, c: MergeCandidate) =>
  `${projectId}:${c.loser_id}>${c.winner_id}`

function ReviewPage() {
  const { t } = useTranslation()
  const toast = useToast()

  const [projects, setProjects] = useState<Project[]>([])
  const [projectFilter, setProjectFilter] = useState<number>(0) // 0 = all
  const [pending, setPending] = useState<WikiPendingPage[]>([])
  const [rejected, setRejected] = useState<WikiPendingPage[]>([])
  const [jobs, setJobs] = useState<CompileJob[]>([])
  const [merge, setMerge] = useState<MergeCandidateReport | null>(null)
  const [dismissed, setDismissed] = useState<string[]>([])
  const [community, setCommunity] = useState<WikiCommunityReport | null>(null)
  const [communityBusy, setCommunityBusy] = useState(false)
  const [loadError, setLoadError] = useState('')
  const [busy, setBusy] = useState(false)
  const [reloadToken, setReloadToken] = useState(0)
  const reload = useCallback(() => setReloadToken(n => n + 1), [])

  useEffect(() => {
    // House pattern: the effect body only wires promises and cleanup — every
    // setState happens inside a .then/.catch. Calling a setState-bearing function
    // directly here is what the react-hooks lint rule rejects, and a synchronous
    // setState on mount would cascade-render the whole page.
    //
    // The merge candidate pass rides along because it is free and structural
    // (ADR-0019 决策 2: no model, no vector, no writes) — a reviewer sees what is
    // waiting before anything is enabled.
    let cancelled = false
    Promise.all([
      getProjects(),
      listPendingWikiPages(projectFilter),
      listRejectedWikiPages(projectFilter),
      listCompileJobs(projectFilter, '', 10),
      findMergeCandidates(projectFilter, mergeTopK),
    ]).then(([projs, pend, rej, jb, cand]) => {
      if (cancelled) return
      setProjects(projs)
      setPending(pend)
      setRejected(rej)
      setJobs(jb)
      setMerge(cand)
      setLoadError('')
    }).catch((e) => {
      // Surface the underlying message next to the generic banner. A bare
      // "加载审核队列失败" once hid the real cause ("unknown method:
      // ListPendingWikiPages" — a stale repotory-server binary predating the
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

  // ADR-0019 决策 1: this click is the only path to the merge transaction in the
  // whole product — no job, no ticker reaches it. The row arms the button first
  // (useConfirmClick), so nothing here fires without a deliberate second click.
  const onMerge = (c: MergeCandidate) => void runGuarded(async () => {
    const r = await mergeWikiPages(c.loser_id, c.winner_id)
    toast({
      kind: 'success',
      title: t('review.mergeDone', { loser: c.loser_slug, winner: c.winner_slug }),
      message: r
        ? t('review.mergeResult', {
          edges: r.edges_redirected,
          dropped: r.edges_dropped,
          notes: r.notes_reattached,
        })
        : t('review.mergeResultUnknown'),
    })
  })

  const onIgnoreMerge = (c: MergeCandidate) =>
    setDismissed(d => (d.includes(mergePairKey(projectFilter, c)) ? d : [...d, mergePairKey(projectFilter, c)]))

  // ADR-0020: slow and paid for, so it is its own in-flight flag rather than
  // riding on `busy` — the page's other buttons should not grey out for it, and
  // its own label has to change. runGuarded still owns the reload and the error
  // toast once the call settles.
  const onRunCommunity = () => {
    setCommunityBusy(true)
    void runGuarded(async () => {
      const r = await runCommunitySummary(projectFilter)
      setCommunity(r)
      if (!r) {
        toast({ kind: 'error', title: t('review.communityNoReport') })
        return
      }
      // Gate off = detection only. Saying "0 pages created" would read as "there
      // is nothing to synthesise", when the real answer is "you never turned this
      // on" — so the backend's own note goes up verbatim instead.
      if (r.community_gate_off) {
        toast({ kind: 'info', title: t('review.communityGateOff'), message: r.llm_note || undefined })
        return
      }
      toast({ kind: 'success', title: t('review.communityDone') })
    }).finally(() => setCommunityBusy(false))
  }

  const mergeCandidates = useMemo(() => {
    const list = merge?.candidates ?? []
    if (dismissed.length === 0) return list
    const hidden = new Set(dismissed)
    return list.filter(c => !hidden.has(mergePairKey(projectFilter, c)))
  }, [merge, dismissed, projectFilter])

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
          {/* Community synthesis (ADR-0020). Same project rule as lint, same
              reason: project_id 0 is the GLOBAL tier on the backend, not
              "every project" — running it there would synthesise a topic out of
              cross-project pages and say nothing about the project the reviewer
              picked, so the button is disabled with that stated, not silently
              aimed at tier 0. */}
          <button
            className="btn btn-sm"
            onClick={onRunCommunity}
            disabled={busy || communityBusy || projectFilter === 0}
            title={projectFilter === 0 ? t('review.communityNeedsProject') : t('review.communityHint')}
          >
            {communityBusy ? t('review.communityRunning') : t('review.startCommunity')}
          </button>
        </div>
      </div>

      <div className={s.content}>
        <p className={s.lead}>{t('review.subtitle')}</p>

        {loadError && <ErrorBanner message={loadError} onRetry={reload} />}

        {/* The last synthesis run stays on screen: its counters are the evidence
            that a paid-for call did something, and a toast expires long before a
            reviewer has read six numbers. */}
        {community && <CommunityReport report={community} />}

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

        {/* Merge suggestions (ADR-0019): always mounted, because "the structural
            pass found nothing" is itself the answer a reviewer wants — unlike the
            job and error sections, which are absence of a thing nobody asked for. */}
        <section>
          <div className={s.sectionHead}>
            <h2>{t('review.mergeHeading')}</h2>
            <span className={s.countChip}>{mergeCandidates.length}</span>
          </div>
          <p className={s.lead}>{t('review.mergeHint')}</p>
          {mergeCandidates.length === 0
            ? <div className="empty-state small">
                <div className="empty-icon"><Icon name="check" size={32} /></div>
                <h3>{t('review.emptyMergeTitle')}</h3>
                <p>{t('review.emptyMergeMsg')}</p>
              </div>
            : <ul className={s.mergeList}>
                {mergeCandidates.map(c => (
                  <MergeRow key={`${c.loser_id}-${c.winner_id}`} item={c} busy={busy}
                    onMerge={onMerge} onIgnore={onIgnoreMerge} />
                ))}
              </ul>}
          {merge?.truncated && (
            <p className={s.note}>{t('review.mergeTruncated', { n: merge.candidates?.length ?? 0 })}</p>
          )}
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

// One suggested pair. The direction is the whole content of the row: [[loser]] →
// [[winner]] is what the merge would do, and it is provisional (ADR-0019 待决 1 —
// structural evidence picked it, not a judgement). The score and the signal names
// are shown rather than a bar because the point of a structural pass is that a
// human can read WHY the pair was suggested; an opaque 0.87 could not be argued
// with. The signals are the backend's stable vocabulary (shared-notes,
// slug-shape, title-overlap, linked) and appear verbatim, like job statuses do.
function MergeRow({
  item, busy, onMerge, onIgnore,
}: {
  item: MergeCandidate
  busy: boolean
  onMerge: (c: MergeCandidate) => void
  onIgnore: (c: MergeCandidate) => void
}) {
  const { t } = useTranslation()
  // Two-click arm: the first click only swaps the label to 确认？, the second one
  // merges. The warning lives on the row itself because that is where the two
  // slugs being destroyed are visible — a modal (or window.confirm, which the
  // delete path above still uses) would take the reviewer away from the very
  // evidence they need in order to decide.
  const { armed, click } = useConfirmClick(() => onMerge(item))
  return (
    <li className={s.mergeRow}>
      <div className={s.mergeTop}>
        <span className={s.mergeDirection}>
          <span className={s.mergeLoser}>{item.loser_slug}</span>
          <span className={s.mergeArrow} aria-hidden="true">→</span>
          <span className={s.mergeWinner}>{item.winner_slug}</span>
          <span className="visually-hidden">
            {t('review.mergeDirectionAria', { loser: item.loser_slug, winner: item.winner_slug })}
          </span>
        </span>
        <span className={s.stat}>{t('review.mergeScore', { n: item.score })}</span>
        {(item.signals ?? []).map(sig => <span key={sig} className={s.signal}>{sig}</span>)}
        {item.shared_notes > 0 && (
          <span className={s.signal}>{t('review.mergeSharedNotes', { n: item.shared_notes })}</span>
        )}
      </div>
      <div className={s.mergeActions}>
        {/* Ignore is local and ephemeral by design: hiding a pair the reviewer has
            already looked at is a view convenience, not a stored decision, and a
            stored one would quietly teach the detector that these are different. */}
        <button className="btn btn-sm" disabled={busy} onClick={() => onIgnore(item)}>
          {t('review.mergeIgnore')}
        </button>
        <button
          className={`btn btn-sm ${armed ? 'btn-danger' : 'btn-primary'} ${s.mergeAdopt}`}
          onClick={click}
          disabled={busy}
          title={t('review.mergeConfirmHint')}
        >
          {armed ? t('review.mergeConfirm') : t('review.mergeAdopt')}
        </button>
      </div>
    </li>
  )
}

// The synthesis report. Every counter is shown even when zero: "0 pages created"
// is a fact about this run, whereas hiding zeros would make a run that found
// nothing indistinguishable from a button that was never pressed.
function CommunityReport({ report }: { report: WikiCommunityReport }) {
  const { t } = useTranslation()
  const gates = [
    { key: 'review.communityPagesCreated', n: report.pages_created },
    { key: 'review.communityPagesUpdated', n: report.pages_updated },
    { key: 'review.communityDocsLinked', n: report.documents_linked },
    { key: 'review.communityRevisionTodos', n: report.revision_todos },
    { key: 'review.communityEmptyBodies', n: report.empty_bodies },
  ]
  return (
    <section aria-label={t('review.communityHeading')}>
      <div className={s.sectionHead}>
        <h2>{t('review.communityHeading')}</h2>
      </div>
      {report.community_gate_off ? (
        // Detection only: the model half never ran, so the generation counters
        // would be zeros nobody earned. The backend's own sentence says why.
        <p className={s.note}>{t('review.communityGateOffNote')}</p>
      ) : (
        <div className={s.jobStats}>
          {gates.map(g => <span key={g.key} className={s.stat}>{t(g.key, { n: g.n })}</span>)}
          <span className={s.stat}>{t('review.communityOutbound', { n: report.outbound_bytes })}</span>
        </div>
      )}
      {/* llm_note is backend copy (why nothing was found, why nothing was sent),
          so it is rendered verbatim rather than paraphrased into a counter. */}
      {report.llm_note && <p className={s.jobNote}>{report.llm_note}</p>}
    </section>
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
  // Page version history (ADR-0021) is lazy and per-card: the list is a second
  // round trip nobody asked for until they opened the row, and one reviewer
  // comparing three pages should not pay for all three histories.
  const [versions, setVersions] = useState<WikiPageVersion[] | null>(null)
  const [versionsBusy, setVersionsBusy] = useState(false)
  const [versionsError, setVersionsError] = useState('')
  const [diff, setDiff] = useState<{ versionId: number; text: string } | null>(null)
  const [diffBusy, setDiffBusy] = useState(false)

  const failure = (e: unknown) => {
    const detail = e instanceof Error ? e.message : String(e)
    setVersionsError(detail ? `${t('review.versionLoadError')}: ${detail}` : t('review.versionLoadError'))
  }

  const toggleVersions = () => {
    if (versions !== null) {
      setVersions(null)
      setDiff(null)
      setVersionsError('')
      return
    }
    setVersionsError('')
    setVersionsBusy(true)
    // Newest first is the backend's contract (ORDER BY id DESC on the page_id
    // index), so it is rendered as returned — re-sorting here would let the two
    // version screens (notes and pages) disagree about what "newest" means.
    listWikiPageVersions(p.id)
      .then(v => setVersions(v))
      .catch(failure)
      .finally(() => setVersionsBusy(false))
  }

  const showDiff = (versionId: number) => {
    if (diff?.versionId === versionId) {
      setDiff(null)
      return
    }
    setVersionsError('')
    setDiffBusy(true)
    diffWikiPageVersion(p.id, versionId)
      .then(text => setDiff({ versionId, text }))
      .catch(failure)
      .finally(() => setDiffBusy(false))
  }

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
          approve/decline decision, so the pair reads as one dialog. The version
          history toggle sits left of them: it is a reading aid, not a verdict,
          and putting it on the right would make it compete with 批准. */}
      <div className={s.actions}>
        <button
          className={`btn btn-sm ${s.versionToggle}`}
          onClick={toggleVersions}
          disabled={versionsBusy}
          aria-expanded={versions !== null}
          title={t('review.versionHistoryHint')}
        >
          {versionsBusy ? t('review.versionLoading') : t('review.versionHistory')}
        </button>
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

      {/* ADR-0021: what a reviewer actually signed off on. The trigger keeps one
          snapshot per approved state, so this is the audit answer to "was it this
          page before?" — and on a rejected card it is the last thing the page said
          while it was still a conclusion. */}
      {versions !== null && (
        <div className={s.versions}>
          <div className={s.versionsHead}>
            <h3>{t('review.versionListTitle')}</h3>
            <button className="btn btn-sm" onClick={toggleVersions}>{t('common.close')}</button>
          </div>
          {versionsError && <p className={s.jobError}>{versionsError}</p>}
          {versions.length === 0 && <p className="empty-hint">{t('review.versionEmpty')}</p>}
          <ul className={s.versionList}>
            {versions.map(v => (
              <li key={v.id}>
                <button
                  className={`${s.versionItem} ${diff?.versionId === v.id ? s.versionItemActive : ''}`}
                  onClick={() => showDiff(v.id)}
                  disabled={diffBusy}
                  title={t('review.versionDiffHint')}
                >
                  <span className={s.versionTime}>{v.created_at}</span>
                  <span className={s.versionKind}>{t(`review.kind_${v.kind}`, v.kind)}</span>
                  <span className={s.versionSource}>{v.source}</span>
                </button>
              </li>
            ))}
          </ul>
          {diff && (
            diff.text
              ? <DiffViewer text={diff.text} />
              : <p className="empty-hint">{t('review.versionNoDiff')}</p>
          )}
        </div>
      )}
    </li>
  )
}

export default ReviewPage
