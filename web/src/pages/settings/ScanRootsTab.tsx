import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { updateScanRoots, type AppConfig, type ScanRootRejection } from '../../api/client'
import s from './ScanRootsTab.module.css'

interface Props {
  data: AppConfig | null
  onChange: (data: AppConfig) => void
  showMessage: (msg: string) => void
}

/**
 * Strip trailing slashes and collapse the rest, so the input box shows the same
 * spelling that will actually be stored. The backend does the authoritative
 * normalisation (filepath.Clean); this only keeps the field from looking like it
 * stored something different from what was typed.
 */
function normalizeInput(raw: string): string {
  const trimmed = raw.trim().replace(/[/\\]+$/, '')
  return trimmed
}

/** Stable, collision-free React key for a root path. */
function rootKey(path: string, index: number): string {
  return `${index}:${path}`
}

export default function ScanRootsTab({ data, onChange, showMessage }: Props) {
  const { t } = useTranslation()
  const [newRoot, setNewRoot] = useState('')
  const [saving, setSaving] = useState(false)

  /**
   * Render a list whose entries are unique. The backend now guarantees this, but
   * the list is also keyed by path, and a duplicate key is a hard React error
   * rather than a cosmetic glitch — so dedupe on the way in as a last line of
   * defence. Keeps an older cached config or a partially-applied write from
   * taking the whole tab down.
   */
  const roots = useMemo(() => {
    const seen = new Set<string>()
    return (data?.scan_roots ?? []).filter((r) => {
      const key = r.toLowerCase()
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
  }, [data?.scan_roots])

  /** Show what the backend refused, and why, instead of silently dropping it. */
  const reportRejections = (rejected?: ScanRootRejection[]) => {
    if (!rejected || rejected.length === 0) return false
    const detail = rejected
      .map((r) => `${r.path} — ${t(`settings.rootRejectReason.${r.reason}`, { defaultValue: r.reason })}`)
      .join('；')
    showMessage(t('settings.rejectedRoots', { msg: detail }))
    return true
  }

  const handleAddRoot = async () => {
    const candidate = normalizeInput(newRoot)
    if (!candidate || !data) return
    setSaving(true)
    try {
      const submitted = [...roots, candidate]
      // Trust the server's list over the locally-assembled one: it may have
      // normalised or refused entries, and mirroring that keeps the rendered
      // list identical to what is persisted.
      const result = await updateScanRoots(submitted)
      const stored = result?.scan_roots ?? submitted
      onChange({ ...data, scan_roots: stored })
      setNewRoot('')
      showMessage(t('settings.added'))
      reportRejections(result?.rejected)
    } catch (e: unknown) {
      showMessage(t('settings.addFailedMsg', { msg: e instanceof Error ? e.message : t('common.unknownError') }))
    } finally {
      setSaving(false)
    }
  }

  const handleRemoveRoot = async (path: string) => {
    if (!data) return
    setSaving(true)
    try {
      const updated = roots.filter((r) => r !== path)
      const result = await updateScanRoots(updated)
      const stored = result?.scan_roots ?? updated
      onChange({ ...data, scan_roots: stored })
      showMessage(t('settings.removed'))
      reportRejections(result?.rejected)
    } catch (e: unknown) {
      showMessage(t('settings.removeFailedMsg', { msg: e instanceof Error ? e.message : t('common.unknownError') }))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="settings-section">
      <h2>{t('settings.scanRoots')}</h2>
      <div className="form-group">
        <label htmlFor="settings-new-root">{t('settings.addRoot')}</label>
        <div className="input-row">
          <input
            id="settings-new-root"
            type="text"
            value={newRoot}
            onChange={(e) => setNewRoot(e.target.value)}
            placeholder="/path/to/your/code"
            className="form-input"
          />
          <button className="btn btn-primary" onClick={handleAddRoot} disabled={saving}>
            {t('settings.add')}
          </button>
        </div>
      </div>
      <ul className={s.rootList}>
        {roots.map((root, i) => (
          <li key={rootKey(root, i)} className={s.rootItem}>
            <span className={s.rootPath}>{root}</span>
            <button className="btn btn-danger btn-sm" onClick={() => handleRemoveRoot(root)} disabled={saving}>
              {t('settings.remove')}
            </button>
          </li>
        ))}
        {roots.length === 0 && (
          <li className={`${s.rootItem} ${s.rootItemEmpty}`}>{t('settings.noRoots')}</li>
        )}
      </ul>
    </div>
  )
}