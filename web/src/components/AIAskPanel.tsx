import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import Icon from './Icon'
import {
  askAI,
  createNoteWithMeta,
  getConfig,
  getProjectDetail,
  getProjectOverview,
} from '../api/client'

interface Props {
  projectId: number
  projectName: string
  onToast: (item: { kind: 'success' | 'error'; title: string; message?: string; duration?: number }) => void
  onGoToSettings: () => void
}

/**
 * AI Q&A for one project, rendered as a tab inside the floating capture panel
 * (it used to be a separate dialog opened from the project header, which put
 * two entry points for the same "write something about this project" intent
 * in two different places and made the AI one unreachable off a project page).
 *
 * The loop is unchanged, and still does not call a model from the app itself
 * unless the user configured an endpoint in Settings → AI:
 *   1. ask a question — either sent straight to the configured endpoint, or
 *      copied as a full prompt pack for any other AI chat;
 *   2. paste/edit the answer;
 *   3. file the Q&A as a knowledge note.
 *
 * The prompt pack is assembled here from the project's own detail + overview,
 * so the panel works for any project the user picks in the dropdown rather
 * than only the one whose page they happen to be on.
 */
export default function AIAskPanel({ projectId, projectName, onToast, onGoToSettings }: Props) {
  const { t } = useTranslation()
  const [question, setQuestion] = useState('')
  const [answer, setAnswer] = useState('')
  const [copied, setCopied] = useState(false)
  const [saving, setSaving] = useState(false)
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

  const sendDirect = async () => {
    if (!question.trim() || asking) return
    setAsking(true)
    try {
      const reply = await askAI(projectId, question.trim())
      setAnswer(reply)
      onToast({ kind: 'success', title: t('ai.replyReceived') })
    } catch (e) {
      onToast({ kind: 'error', title: t('ai.sendFailed'), message: e instanceof Error ? e.message : undefined })
    } finally {
      setAsking(false)
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

  return (
    <div className="ai-ask-body">
      <p className="ai-hint">
        {t('ai.hint')}
        {projectName && <span className="ai-hint-project"> · {projectName}</span>}
      </p>
      {configured === false && (
        <p className="ai-hint ai-hint-config">
          {t('ai.notConfigured')}{' '}
          <button className="link-btn" onClick={onGoToSettings}>
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

      <div className="ai-actions">
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

      <textarea
        className="fab-content form-input ai-answer"
        rows={8}
        value={answer}
        onChange={e => setAnswer(e.target.value)}
        placeholder={t('ai.answerPlaceholder')}
        aria-label={t('ai.answer')}
      />

      <div className="ai-actions">
        <button className="btn btn-primary btn-sm" onClick={saveAsNote} disabled={!canSave}>
          {saving ? t('fab.saving') : t('ai.saveAsNote')}
        </button>
      </div>
    </div>
  )
}
