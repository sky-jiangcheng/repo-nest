import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { listAIModels, testAIChat, updateConfig } from '../../api/client'

interface Props {
  /** Flat key/value config record (data.config from GetConfig). */
  config: Record<string, string>
  showMessage: (msg: string) => void
  onSaved: () => void
}

/**
 * AI chat endpoint configuration: any OpenAI-compatible /chat/completions
 * server works — a local LM Studio (http://localhost:1234/v1), vLLM, Ollama,
 * or a remote provider. Base URL can be probed (model list), the model picked
 * from what the server reports, and the model itself tested with a tiny
 * completion — all against the typed values, no save required. The API key
 * is optional (local servers usually need none), stored server-side, and
 * redacted on read.
 */
export default function AiTab({ config, showMessage, onSaved }: Props) {
  const { t } = useTranslation()
  const [baseURL, setBaseURL] = useState(config.ai_chat_base_url || '')
  const [model, setModel] = useState(config.ai_chat_model || '')
  const [apiKey, setApiKey] = useState(config.ai_chat_api_key || '')
  const [saving, setSaving] = useState(false)
  const [models, setModels] = useState<string[]>([])
  const [modelsLoading, setModelsLoading] = useState(false)
  const [modelsMsg, setModelsMsg] = useState('')
  const [testMsg, setTestMsg] = useState('')
  const [testing, setTesting] = useState(false)

  const fetchModels = async () => {
    if (!baseURL.trim() || modelsLoading) return
    setModelsLoading(true)
    setModelsMsg('')
    setModels([])
    try {
      const res = await listAIModels(baseURL.trim(), apiKey.trim())
      setModels(res.models)
      // The probe tries near-miss path variants (LM Studio's base is /v1 but
      // ".../api/v1" is the classic mistype); adopt the URL that worked.
      if (res.resolved_base_url && res.resolved_base_url !== baseURL.trim()) {
        setBaseURL(res.resolved_base_url)
        setModelsMsg(t('settings.ai.modelsFoundResolved', { count: res.models.length, url: res.resolved_base_url }))
      } else {
        setModelsMsg(t('settings.ai.modelsFound', { count: res.models.length }))
      }
    } catch (e: unknown) {
      setModelsMsg(t('settings.ai.fetchFail', { msg: e instanceof Error ? e.message : t('common.unknownError') }))
    } finally {
      setModelsLoading(false)
    }
  }

  const testModel = async () => {
    if (!model.trim() || testing) return
    setTesting(true)
    setTestMsg('')
    try {
      const res = await testAIChat(baseURL.trim(), model.trim(), apiKey.trim())
      if (res.resolved_base_url && res.resolved_base_url !== baseURL.trim()) {
        setBaseURL(res.resolved_base_url)
      }
      setTestMsg(t('settings.ai.testOk', { reply: res.reply.slice(0, 80) }))
    } catch (e: unknown) {
      setTestMsg(t('settings.ai.testFail', { msg: e instanceof Error ? e.message : t('common.unknownError') }))
    } finally {
      setTesting(false)
    }
  }

  const save = async (): Promise<boolean> => {
    if (!baseURL.trim() || !model.trim()) {
      showMessage(t('settings.ai.missingFields'))
      return false
    }
    setSaving(true)
    try {
      await updateConfig('ai_chat_base_url', baseURL.trim())
      await updateConfig('ai_chat_model', model.trim())
      // The masked placeholder round-trips as "unchanged"; only overwrite the
      // secret when the user typed a fresh value.
      if (apiKey.trim() && apiKey !== '********') {
        await updateConfig('ai_chat_api_key', apiKey.trim())
      }
      showMessage(t('settings.configSaved'))
      onSaved()
      return true
    } catch (e: unknown) {
      showMessage(t('settings.saveFailedMsg', { msg: e instanceof Error ? e.message : t('common.unknownError') }))
      return false
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="settings-section">
      <h2>{t('settings.ai.title')}</h2>
      <p className="section-desc">{t('settings.ai.desc')}</p>

      <div className="form-group">
        <label htmlFor="ai-base-url">{t('settings.ai.baseUrl')}</label>
        <div className="ai-url-row">
          <input
            id="ai-base-url"
            type="text"
            value={baseURL}
            onChange={(e) => setBaseURL(e.target.value)}
            className="form-input"
            placeholder="http://localhost:1234/v1"
          />
          <button
            className="btn btn-secondary"
            onClick={fetchModels}
            disabled={modelsLoading || !baseURL.trim()}
          >
            {modelsLoading ? t('settings.ai.fetching') : t('settings.ai.fetchModels')}
          </button>
        </div>
        <span className="form-hint">{t('settings.ai.baseUrlHint')}</span>
        {modelsMsg && <span className="form-hint">{modelsMsg}</span>}
      </div>

      <div className="form-group">
        <label htmlFor="ai-model">{t('settings.ai.model')}</label>
        <input
          id="ai-model"
          type="text"
          value={model}
          onChange={(e) => setModel(e.target.value)}
          className="form-input"
          list="ai-model-options"
          placeholder="qwen2.5-7b-instruct / gpt-4o-mini / …"
        />
        <datalist id="ai-model-options">
          {models.map(m => <option key={m} value={m} />)}
        </datalist>
        <span className="form-hint">{t('settings.ai.modelHint')}</span>
      </div>

      <div className="form-group">
        <label htmlFor="ai-api-key">{t('settings.ai.apiKeyOptional')}</label>
        <input
          id="ai-api-key"
          type="password"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          className="form-input"
          placeholder={t('settings.ai.apiKeyPlaceholder')}
        />
        <span className="form-hint">{t('settings.ai.apiKeyHint')}</span>
      </div>

      <div className="empty-actions">
        <button className="btn btn-primary" onClick={testModel} disabled={testing || !model.trim()}>
          {testing ? t('settings.ai.testing') : t('settings.ai.testModel')}
        </button>
        <button className="btn btn-secondary" onClick={save} disabled={saving || testing}>
          {saving ? t('settings.saving', { defaultValue: 'Saving…' }) : t('settings.save', { defaultValue: 'Save' })}
        </button>
      </div>
      {testMsg && <p className="form-hint">{testMsg}</p>}
    </div>
  )
}
