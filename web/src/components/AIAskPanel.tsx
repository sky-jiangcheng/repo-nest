import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import Icon from './Icon'
import {
  askAIWithEvidenceStream,
  createNoteWithMeta,
  fileAnswerAsPage,
  getConfig,
  getProjectDetail,
  getProjectOverview,
  type Evidence,
} from '../api/client'
import s from './AIAskPanel.module.css'
import type { PushToast } from '../hooks/useToast'

interface Props {
  projectId: number
  projectName: string
  onToast: PushToast
  onGoToSettings: () => void
}

/**
 * AI Q&A for one project, rendered as a tab inside the floating capture panel
 * (it used to be a separate dialog opened from the project header, which put
 * two entry points for the same "write something about this project" intent
 * in two different places and made the AI one unreachable off a project page).
 *
 * Since ADR-0014 M6-W2 the direct mode runs on retrieved evidence: the reply
 * comes back with the pages and notes that were put in front of the model, the
 * panel shows them, and filing an answer can link to exactly what it cited.
 * The loop itself is unchanged:
 *   1. ask — answered by the configured endpoint with a ranked, budgeted evidence
 *      block, or copied as a full prompt pack for any other AI chat;
 *   2. paste/edit the answer;
 *   3. file it as a knowledge note, or as a `query` page that links its citations.
 *
 * The prompt pack is still assembled here from the project's own detail +
 * overview, so the tab works for any project picked in the dropdown rather than
 * only the one whose page they happen to be on.
 */
export default function AIAskPanel({ projectId, projectName, onToast, onGoToSettings }: Props) {
  const { t } = useTranslation()
  const [question, setQuestion] = useState('')
  const [answer, setAnswer] = useState('')
  // Evidence carries the project it was retrieved for. Refs are positional labels
  // of one retrieval, so an "P1" from another project would resolve onto whatever
  // page happens to hold that label here — tagging beats clearing-in-an-effect,
  // which the repo's react-hooks lint rule (rightly) forbids.
  const [evidence, setEvidence] = useState<{ projectId: number; data: Evidence } | null>(null)
  const [copied, setCopied] = useState(false)
  const [saving, setSaving] = useState(false)
  const [filing, setFiling] = useState(false)
  const [configured, setConfigured] = useState<boolean | null>(null)
  const [asking, setAsking] = useState(false)
  const [contextLines, setContextLines] = useState<string[] | null>(null)
  const qRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    qRef.current?.focus()
  }, [])

  // Direct mode needs a configured chat endpoint; without one the tab still
  // works through the copy-prompt-pack route.
  useEffect(() => {
    let cancelled = false
    getConfig().then(d => {
      if (cancelled) return
      const c = d.config || {}
      setConfigured(!!(c.ai_chat_base_url && c.ai_chat_model))
    }).catch(() => { if (!cancelled) setConfigured(false) })
    return () => { cancelled = true }
  }, [])

  // Context is fetched for whichever project is selected, and only when the
  // user actually asks — building it eagerly on tab open would cost two
  // round trips for a panel that may never be used.
  const ensureContext = async (): Promise<string[]> => {
    if (contextLines) return contextLines
    const [detail, overview] = await Promise.all([
      getProjectDetail(projectId),
      getProjectOverview(projectId).catch(() => null),
    ])
    const lines: string[] = []
    lines.push(`Project: ${detail.name}`)
    lines.push(`Path: ${detail.root_path}`)
    const repos = detail.repos || []
    if (repos.length) {
      lines.push('', `Repositories (${repos.length}):`)
      for (const r of repos.slice(0, 30)) lines.push(`- ${r.path}`)
      if (repos.length > 30) lines.push(`- ...and ${repos.length - 30} more`)
    }
    if (overview?.tech_stack?.length) {
      lines.push('', 'Tech stack:', overview.tech_stack.map(x => x.name).join(', '))
    }
    if (overview?.activity && (overview.activity.total_commits > 0 || overview.activity.last_commit_date)) {
      lines.push(
        '',
        `## Activity\n- total commits: ${overview.activity.total_commits}\n- active days (90d): ${overview.activity.active_days}\n- commits (30d): ${overview.activity.commit_rate_30d}\n- last commit: ${overview.activity.last_commit_date}`,
      )
    }
    if (overview?.recent_commits?.length) {
      lines.push('', `## ${t('project.recentCommits')}\n${overview.recent_commits.slice(0, 5).map(c => `- ${c.time} ${c.message}`).join('\n')}`)
    }
    setContextLines(lines)
    return lines
  }

  // Streaming direct answer: the reply renders as it forms instead of the panel
  // sitting on 询问中… for a multi-minute wait. Both transports resolve to the
  // same EvidenceAnswer shape, so evidence/truncated handling is unchanged.
  const sendDirect = () => {
    if (!question.trim() || asking) return
    setAsking(true)
    setAnswer('')
    setEvidence(null)
    askAIWithEvidenceStream(projectId, question.trim(), {
      onDelta: delta => setAnswer(prev => prev + delta),
      onDone: res => {
        setEvidence({ projectId, data: res.evidence })
        setAsking(false)
        const n = res.evidence?.items?.length ?? 0
        // A truncated reply is reported as a warning, not folded into the success
        // toast: the answer did arrive and is usable, but it is a prefix, and the
        // one remedy (bigger output budget / different model) is not something the
        // user would guess from "收到回复".
        onToast(res.truncated
          ? { kind: 'info', title: t('ai.replyTruncated'), message: t('ai.replyTruncatedHint') }
          : n > 0
            ? { kind: 'success', title: t('ai.replyReceived'), message: t('ai.evidenceCount', { n }) }
            : { kind: 'success', title: t('ai.replyReceived'), message: t('ai.evidenceNone') })
      },
      onError: e => {
        setAsking(false)
        onToast({ kind: 'error', title: t('ai.sendFailed'), message: e instanceof Error ? e.message : undefined })
      },
    })
  }

  // null when the stored evidence belongs to another project: nothing may be filed
  // from it and nothing is shown for it.
  const liveEvidence = evidence && evidence.projectId === projectId ? evidence.data : null
  const items = liveEvidence?.items ?? []

  // Refs are positional labels of the retrieval that produced this answer, so the
  // answer's own citations are what filing may link to. Falling back to the whole
  // evidence set when the model cited nothing keeps provenance honest in the
  // common case (a cited answer files its citations) without turning an uncited
  // answer into an unfileable one: it records "answered from this batch" instead.
  const citedRefs = useMemo(() => {
    const all = (evidence && evidence.projectId === projectId ? evidence.data.items : []).map(i => i.ref)
    const found = Array.from(new Set(
      (answer.match(/\[[PN]\d+\]/g) ?? []).map(x => x.slice(1, -1).toUpperCase()),
    ))
    if (found.length === 0) return all
    // Keep only labels this retrieval actually issued; a hallucinated [P9] must not
    // resolve, and the backend drops it too — filtering here keeps the count right.
    return found.filter(r => all.includes(r))
  }, [answer, evidence, projectId])

  const fileAsPage = async () => {
    if (!question.trim() || !answer.trim() || !liveEvidence || filing) return
    setFiling(true)
    try {
      const pageId = await fileAnswerAsPage(
        projectId, question.trim(), answer.trim(), liveEvidence, citedRefs)
      onToast({
        kind: 'success',
        title: t('ai.pageSaved'),
        message: t('ai.pageSavedDetail', { id: pageId, refs: citedRefs.length }),
      })
    } catch (e) {
      onToast({ kind: 'error', title: t('ai.pageSaveFailed'), message: e instanceof Error ? e.message : undefined })
    } finally {
      setFiling(false)
    }
  }

  const copyPrompt = async () => {
    if (!question.trim()) return
    try {
      const lines = await ensureContext()
      const pack = `${t('ai.promptHeader', { defaultValue: 'Question' })}: ${question.trim()}\n\n${lines.join('\n')}`
      await navigator.clipboard.writeText(pack)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
      onToast({ kind: 'success', title: t('ai.promptCopied') })
    } catch {
      onToast({ kind: 'error', title: t('common.failed') })
    }
  }

  const saveAsNote = async () => {
    if (!question.trim() || !answer.trim() || saving) return
    setSaving(true)
    try {
      const content = `## ${t('ai.questionLabel')}\n\n${question.trim()}\n\n## ${t('ai.answerLabel')}\n\n${answer.trim()}`
      await createNoteWithMeta(projectId, content, {
        title: question.trim().split('\n')[0].slice(0, 80),
        kind: 'knowledge',
        source: 'ai',
      })
      onToast({ kind: 'success', title: t('ai.saved') })
    } catch (e) {
      onToast({ kind: 'error', title: t('fab.saveFailed'), message: e instanceof Error ? e.message : undefined })
    } finally {
      setSaving(false)
    }
  }

  const canCopy = question.trim() !== ''
  const canSave = question.trim() !== '' && answer.trim() !== '' && !saving
  const canFile = canSave && items.length > 0

  return (
    <div className={s.aiAskBody}>
      <p className={s.aiHint}>
        {t('ai.hint')}
        {projectName && <span className={s.aiHintProject}> · {projectName}</span>}
      </p>
      {configured === false && (
        <p className={`${s.aiHint} ${s.aiHintConfig}`}>
          {t('ai.notConfigured')}{' '}
          <button className={s.linkBtn} onClick={onGoToSettings}>
            {t('ai.goSettings')}
          </button>
        </p>
      )}

      <textarea
        ref={qRef}
        className="fab-content form-input"
        rows={2}
        value={question}
        onChange={e => setQuestion(e.target.value)}
        placeholder={t('ai.questionPlaceholder')}
        aria-label={t('ai.question')}
      />

      <div className={s.aiActions}>
        {configured && (
          <button className="btn btn-primary btn-sm" onClick={sendDirect} disabled={!canCopy || asking}>
            {asking ? t('ai.asking') : t('ai.send')}
          </button>
        )}
        <button className="btn btn-secondary btn-sm" onClick={copyPrompt} disabled={!canCopy}>
          <Icon name={copied ? 'check' : 'file-text'} size={12} />
          {copied ? t('project.copied') : t('ai.copyPrompt')}
        </button>
      </div>

      {liveEvidence && (
        <div className={s.evidence}>
          <div className={s.evidenceHead}>
            <span>{t('ai.evidenceTitle')}</span>
            {items.length === 0 && <span className={s.evidenceNone}>{t('ai.evidenceNone')}</span>}
            {items.length > 0 && (
              <span className={s.evidenceMeta}>
                {t('ai.evidenceCount', { n: items.length })}
                {liveEvidence.truncated && liveEvidence.dropped > 0 && (
                  <span className={s.evidenceDropped}>{t('ai.evidenceDropped', { n: liveEvidence.dropped })}</span>
                )}
              </span>
            )}
          </div>
          {items.length > 0 && (
            <ul className={s.evidenceList}>
              {items.map(it => (
                <li key={`${it.ref}-${it.id}`} className={s.evidenceRow}>
                  <span className={s.evidenceRef}>{`[${it.ref}]`}</span>
                  <span className={s.evidenceKind}>{it.type === 'page' ? it.kind : 'note'}</span>
                  <span className={s.evidenceTitle} title={it.slug ? `${it.title} · ${it.slug}` : it.title}>
                    {it.title}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      <textarea
        className={`fab-content form-input ${s.aiAnswer}`}
        rows={8}
        value={answer}
        onChange={e => setAnswer(e.target.value)}
        placeholder={t('ai.answerPlaceholder')}
        aria-label={t('ai.answer')}
      />

      <div className={s.aiActions}>
        <button className="btn btn-primary btn-sm" onClick={saveAsNote} disabled={!canSave}>
          {saving ? t('fab.saving') : t('ai.saveAsNote')}
        </button>
        {liveEvidence && items.length > 0 && (
          <button
            className="btn btn-secondary btn-sm"
            onClick={fileAsPage}
            disabled={!canFile}
            title={t('ai.saveAsPageHint')}
          >
            {filing ? t('fab.saving') : t('ai.saveAsPage')}
            {citedRefs.length > 0 && <span className={s.fileCount}>{citedRefs.length}</span>}
          </button>
        )}
      </div>
    </div>
  )
}
