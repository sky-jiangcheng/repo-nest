import { useTranslation } from 'react-i18next'
import type { NoteVersion } from '../../api/client'
import DiffViewer from '../DiffViewer'
import s from './VersionHistoryPanel.module.css'

interface Props {
  versions: NoteVersion[]
  restoringId: number | null
  diffText: string | null
  onRestore: (versionId: number) => void
  onShowDiff: (versionId: number) => void
  onClose: () => void
}

/** Version history list for one note, with per-version diff and restore. */
export default function VersionHistoryPanel({ versions, restoringId, diffText, onRestore, onShowDiff, onClose }: Props) {
  const { t } = useTranslation()

  return (
    <div className={s.versionHistoryPanel}>
      <div className={s.versionHistoryHeader}>
        <h4>{t('project.versionHistory')}</h4>
        <button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>
      </div>
      {versions.length === 0 ? (
        <p className="empty-hint">{t('project.noVersions')}</p>
      ) : (
        <div className={s.versionList}>
          {versions.map(v => (
            <div key={v.id} className={s.versionItem}>
              <span className={s.versionTime}>{v.created_at}</span>
              <span className={s.versionTitle}>{v.title || t('project.untitled')}</span>
              <div className={s.versionActions}>
                <button className="btn btn-sm" onClick={() => onShowDiff(v.id)} title={t('project.viewDiff')}>
                  {t('project.diff')}
                </button>
                <button
                  className="btn btn-sm btn-primary"
                  onClick={() => onRestore(v.id)}
                  disabled={restoringId === v.id}
                  title={t('project.restoreVersion', { defaultValue: '恢复到此版本' })}
                >
                  {restoringId === v.id ? t('project.restoring') : t('project.restore')}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
      {diffText !== null && <DiffViewer text={diffText} />}
    </div>
  )
}
