import { useTranslation } from 'react-i18next'

export type Scope = 'week' | 'month' | 'all'

interface Props {
  scope: Scope
  onChange: (s: Scope) => void
}

const OPTIONS: Scope[] = ['week', 'month', 'all']

export default function ScopeToggle({ scope, onChange }: Props) {
  const { t } = useTranslation()
  return (
    <div className="range-toggle" role="group" aria-label={t('project.scope')}>
      {OPTIONS.map((opt) => (
        <button
          key={opt}
          type="button"
          className={`btn btn-sm seg-chip ${scope === opt ? 'seg-chip-active' : ''}`}
          aria-pressed={scope === opt}
          onClick={() => onChange(opt)}
        >
          {t(`project.range${opt[0].toUpperCase()}${opt.slice(1)}`)}
        </button>
      ))}
    </div>
  )
}
