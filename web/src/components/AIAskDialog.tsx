import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Icon from './Icon'
import { askAI, createNoteWithMeta, getConfig } from '../api/client'

interface Props {
  onClose: () => void
  onToast: (item: { kind: 'success' | 'error'; title: string; message?: string; duration?: number }) => void
  /** Builds the full prompt pack (project context + the user's question). */
  buildPrompt: (question: string) => string
  /** Project the resulting note is filed under. */
  projectId: number
}

/**
 * AI Q&A loop, fully inside the product's verified capabilities:
 *   1. ask a question — "copy prompt pack" puts the question plus the whole
 *      generated project context on the clipboard for any AI chat;
 *   2. paste the answer back and correct it freely;
 *   3. optionally file the corrected Q&A as a knowledge note.
 * No model is called from the app itself — RepoNest's role is context
 * packaging, capture, and filing.
 */
export default function AIAskDialog({ onClose, onToast, buildPrompt, projectId }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [question, setQuestion] = useState('')
  const [answer, setAnswer] = useState('')
  const [copied, setCopied] = useState(false)
  const [saving, setSaving] = useState(false)
  const [configured, setConfigured] = useState<boolean | null>(null)
  const [asking, setAsking] = useState(false)
  const qRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    qRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  // Direct mode needs a configured chat endpoint; without one the panel still
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
      await navigator.clipboard.writeText(buildPrompt(question.trim()))
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
      onClose()
    } catch (e) {
      onToast({ kind: 'error', title: t('fab.saveFailed'), message: e instanceof Error ? e.message : undefined })
      setSaving(false)
    }
  }

  const canCopy = question.trim() !== ''
  const canSave = question.trim() !== '' && answer.trim() !== '' && !saving

  return (
    <>
      <div className="fab-backdrop" onClick={onClose} />
      <div className="fab-panel ai-ask-panel" role="dialog" aria-label={t('ai.title')}>
        <div className="fab-panel-head">
          <span className="fab-title">{t('ai.title')}</span>
          <button className="fab-close" onClick={onClose} aria-label={t('common.close', { defaultValue: 'Close' })}>
            <Icon name="close" size={14} />
          </button>
        </div>

        <p className="ai-hint">{t('ai.hint')}</p>
        {configured === false && (
          <p className="ai-hint ai-hint-config">
            {t('ai.notConfigured')}{' '}
            <button className="link-btn" onClick={() => { onClose(); navigate('/settings?tab=ai') }}>
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
    </>
  )
}
