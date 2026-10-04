import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  getKnowledgeSources, triggerKnowledgeImport, captureClaudeHandoff,
  updateConfig, type SourceStatus,
} from '../../api/client'

// B-end / global-memory sources that need an explicit target project (their
// memory is agent-global, not per-project). Value = project name or numeric id.
const TARGET_PROJECT_KEYS = ['openclaw_project', 'hermes_project'] as const

interface Props {
  initialAutoImport: boolean
  initialClaudeCapture: boolean
  initialTargets: Record<string, string>
  showMessage: (msg: string) => void
}

export default function PluginsTab({ initialAutoImport, initialClaudeCapture, initialTargets, showMessage }: Props) {
  const { t } = useTranslation()
  const [sources, setSources] = useState<SourceStatus[]>([])
  const [importingSource, setImportingSource] = useState('')
  const [autoImport, setAutoImport] = useState(initialAutoImport)
  const [claudeCapture, setClaudeCapture] = useState(initialClaudeCapture)
  const [targets, setTargets] = useState<Record<string, string>>(
    Object.fromEntries(TARGET_PROJECT_KEYS.map((k) => [k, initialTargets[k] ?? ''])),
  )
  const [captureProjectId, setCaptureProjectId] = useState('')
  const [saving, setSaving] = useState(false)
  const [capturing, setCapturing] = useState(false)

  useEffect(() => {
    getKnowledgeSources().then(setSources).catch(() => {})
  }, [])

  const handleImportSource = async (name: string) => {
    setImportingSource(name)
    try {
      const r = await triggerKnowledgeImport(name)
      showMessage(t('settings.importDone', { created: r.created, updated: r.updated, skipped: r.skipped }))
    } catch (e: unknown) {
      showMessage(t('settings.importFailed', { msg: e instanceof Error ? e.message : t('common.unknownError') }))
    } finally {
      setImportingSource('')
    }
  }

  const persist = async (key: string, value: string, ok: string) => {
    setSaving(true)
    try {
      await updateConfig(key, value)
      showMessage(ok)
    } catch (e: unknown) {
      showMessage(t('settings.saveFailedMsg', { msg: e instanceof Error ? e.message : t('common.unknownError') }))
    } finally {
      setSaving(false)
    }
  }

  const handleAutoImportToggle = (on: boolean) => {
    setAutoImport(on)
    void persist('auto_import', on ? '1' : '0', on ? t('settings.autoImportOn') : t('settings.autoImportOff'))
  }

  const handleClaudeCaptureToggle = (on: boolean) => {
    setClaudeCapture(on)
    void persist('claude_session_capture', on ? '1' : '0', on ? t('settings.claudeCapOn') : t('settings.claudeCapOff'))
  }

  const handleTargetSave = (key: string) => {
    void persist(key, (targets[key] ?? '').trim(), t('settings.targetSaved', { source: key.replace('_project', '') }))
  }

  const handleCapture = async () => {
    const pid = Number(captureProjectId)
    if (!Number.isInteger(pid) || pid <= 0) {
      showMessage(t('settings.captureNeedProject'))
      return
    }
    setCapturing(true)
    try {
      const r = await captureClaudeHandoff(pid)
      showMessage(t('settings.captureDone', { title: r.title }))
    } catch (e: unknown) {
      showMessage(t('settings.captureFailed', { msg: e instanceof Error ? e.message : t('common.unknownError') }))
    } finally {
      setCapturing(false)
    }
  }

  return (
    <div className="settings-section">
      <p className="section-desc" dangerouslySetInnerHTML={{ __html: t('settings.pluginDesc') }} />

      {/* --- Knowledge import --- */}
      <section className="settings-group">
        <h2 className="settings-group-title">{t('settings.groupImport')}</h2>
        <div className="form-group">
          <label id="settings-auto-import-label">{t('settings.autoImportLabel')}</label>
          <div className="toggle-row">
            <button
              className={`toggle ${autoImport ? 'toggle-on' : ''}`}
              onClick={() => handleAutoImportToggle(!autoImport)}
              disabled={saving}
              aria-pressed={autoImport}
              aria-labelledby="settings-auto-import-label"
            >
              <span className="toggle-knob" />
            </button>
            <span className="form-hint" style={{ marginTop: 0 }}>
              {autoImport ? t('settings.autoImportOnHint') : t('settings.autoImportOffHint')}
            </span>
          </div>
        </div>
        {sources.length === 0 ? (
          <div className="empty-hint">{t('settings.noSources')}</div>
        ) : (
          <ul className="plugin-list">
            {sources.map((s) => (
              <li key={s.name} className="plugin-item plugin-ok">
                <div className="plugin-info">
                  <span className="plugin-name">{s.name}</span>
                  <span className="plugin-path">{t('settings.fromPlugin', { name: s.plugin || 'builtin' })}</span>
                </div>
                {/* Secondary, not primary: five identical filled-blue CTAs in a
                    row read as one giant primary action and none of them wins.
                    The row's hover state carries the emphasis instead. */}
                <button
                  className="btn btn-sm plugin-import-btn"
                  onClick={() => handleImportSource(s.name)}
                  disabled={importingSource !== '' || !s.enabled}
                >
                  {importingSource === s.name ? t('settings.importing') : t('settings.importNow')}
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {/* --- Global-memory sources: target project --- */}
      <section className="settings-group">
        <h2 className="settings-group-title">{t('settings.groupTargets')}</h2>
        <p className="form-hint" style={{ marginTop: 0, marginBottom: 12 }}>{t('settings.targetProjectHint')}</p>
        {TARGET_PROJECT_KEYS.map((key) => (
          <div className="form-group" key={key}>
            <label htmlFor={`target-${key}`}>{t('settings.targetProjectLabel', { source: key.replace('_project', '') })}</label>
            <div className="input-row">
              <input
                id={`target-${key}`}
                className="form-input"
                type="text"
                placeholder={t('settings.targetProjectPlaceholder')}
                value={targets[key] ?? ''}
                onChange={(e) => setTargets((prev) => ({ ...prev, [key]: e.target.value }))}
                disabled={saving}
              />
              <button className="btn btn-secondary btn-sm" onClick={() => handleTargetSave(key)} disabled={saving}>
                {t('settings.save')}
              </button>
            </div>
          </div>
        ))}
      </section>

      {/* --- Claude session capture --- */}
      <section className="settings-group">
        <h2 className="settings-group-title">{t('settings.groupCapture')}</h2>
        <div className="form-group">
          <label id="settings-claude-capture-label">{t('settings.claudeCapLabel')}</label>
          <div className="toggle-row">
            <button
              className={`toggle ${claudeCapture ? 'toggle-on' : ''}`}
              onClick={() => handleClaudeCaptureToggle(!claudeCapture)}
              disabled={saving}
              aria-pressed={claudeCapture}
              aria-labelledby="settings-claude-capture-label"
            >
              <span className="toggle-knob" />
            </button>
            <span className="form-hint" style={{ marginTop: 0 }}>
              {claudeCapture ? t('settings.claudeCapOnHint') : t('settings.claudeCapOffHint')}
            </span>
          </div>
        </div>
        <div className="form-group">
          <label htmlFor="capture-project-id">{t('settings.captureProjectLabel')}</label>
          <div className="input-row">
            <input
              id="capture-project-id"
              className="form-input"
              type="number"
              min={1}
              inputMode="numeric"
              placeholder={t('settings.captureProjectPlaceholder')}
              value={captureProjectId}
              onChange={(e) => setCaptureProjectId(e.target.value)}
              disabled={!claudeCapture || capturing}
            />
            <button className="btn btn-secondary btn-sm" onClick={handleCapture} disabled={!claudeCapture || capturing}>
              {capturing ? t('settings.capturing') : t('settings.captureBtn')}
            </button>
          </div>
        </div>
      </section>
    </div>
  )
}
