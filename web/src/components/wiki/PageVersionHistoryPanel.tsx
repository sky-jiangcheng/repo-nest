import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { diffPageVersions, listPageVersions, restorePageVersion } from '../../api/endpoints'
import type { PageVersion } from '../../api/types'
import { useToast } from '../../hooks/useToast'
import s from './PageVersionHistoryPanel.module.css'

interface Props {
  pageId: number
  onClose: () => void
  /** Called after a successful restore so the caller can refresh the page. */
  onRestored: () => void
}

/**
 * Version history for one wiki page (ADR-0021): snapshots, per-version diff and
 * restore. The page-side twin of the notes' VersionHistoryPanel, with one
 * addition the note panel does not need: a restore confirmation, because a
 * rollback overwrites the current content — which is preserved as its own
 * version, and the confirmation says so (ADR-0021 待决 6).
 */
export default function PageVersionHistoryPanel({ pageId, onClose, onRestored }: Props) {
  const { t } = useTranslation()
  const toast = useToast()
  const [versions, setVersions] = useState<PageVersion[] | null>(null)
  const [restoringId, setRestoringId] = useState<number | null>(null)
  const [confirmingId, setConfirmingId] = useState<number | null>(null)
  const [diffText, setDiffText] = useState<string | null>(null)

  const load = useCallback(() => {
    void listPageVersions(pageId).then(setVersions)
  }, [pageId])

  useEffect(load, [load])

  const showDiff = (versionId: number) => {
    void diffPageVersions(pageId, versionId).then(setDiffText)
  }

  const restore = (versionId: number) => {
    setRestoringId(versionId)
    void restorePageVersion(pageId, versionId)
      .then(() => {
        setConfirmingId(null)
        setDiffText(null)
        toast({ kind: 'success', title: t('review.pageVersionRestored') })
        load()
        onRestored()
      })
      .catch(e => toast({ kind: 'error', title: t('review.actionError'), message: e instanceof Error ? e.message : undefined }))
      .finally(() => setRestoringId(null))
  }

  return (
    <div className={s.panel}>
      <div className={s.header}>
        <h4>{t('review.pageVersionHistory')}</h4>
        <button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>
      </div>
      {versions === null ? (
        <p className="empty-hint">{t('common.loading')}</p>
      ) : versions.length === 0 ? (
        <p className="empty-hint">{t('review.noPageVersions')}</p>
      ) : (
        <div className={s.list}>
          {versions.map(v => (
            <div key={v.id} className={s.item}>
              <span className={s.time}>{v.created_at}</span>
              <span className={s.title}>{v.title || t('review.untitled')}</span>
              <div className={s.actions}>
                <button className="btn btn-sm" onClick={() => showDiff(v.id)} title={t('review.viewDiff')}>
                  {t('review.diff')}
                </button>
                {confirmingId === v.id ? (
                  <>
                    {/* Two-step on purpose: a rollback overwrites the current
                        content, so the confirm step states what happens to it
                        (preserved as a new version) instead of just asking. */}
                    <button
                      className="btn btn-sm btn-primary"
                      disabled={restoringId === v.id}
                      onClick={() => restore(v.id)}
                    >
                      {restoringId === v.id ? t('review.restoring') : t('review.confirmRestore')}
                    </button>
                    <button className="btn btn-sm" onClick={() => setConfirmingId(null)}>
                      {t('common.cancel')}
                    </button>
                  </>
                ) : (
                  <button
                    className="btn btn-sm btn-primary"
                    onClick={() => setConfirmingId(v.id)}
                    title={t('review.restorePageVersion')}
                  >
                    {t('review.restore')}
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
      {diffText !== null && (
        <div className={s.diffPanel}>
          <pre className={s.diffPre}>
            {diffText.split('\n').map((line, i) => {
              if (line.startsWith('+')) {
                return <code key={i} className={s.diffLineAdd}>{line}</code>
              }
              if (line.startsWith('-')) {
                return <code key={i} className={s.diffLineDel}>{line}</code>
              }
              return <code key={i}>{line}</code>
            })}
          </pre>
        </div>
      )}
    </div>
  )
}
