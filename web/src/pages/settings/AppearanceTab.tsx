import { useTranslation } from 'react-i18next'
import { applyTheme, storeTheme, type ThemeMode } from '../../utils/theme'
import s from './AppearanceTab.module.css'

interface Props {
  themeMode: ThemeMode
  onThemeChange: (mode: ThemeMode) => void
  showMessage: (msg: string) => void
}

const THEME_OPTIONS: { value: ThemeMode; labelKey: string; descKey: string }[] = [
  { value: 'light', labelKey: 'settings.themes.light', descKey: 'settings.themes.lightDesc' },
  { value: 'dark', labelKey: 'settings.themes.dark', descKey: 'settings.themes.darkDesc' },
  { value: 'system', labelKey: 'settings.themes.system', descKey: 'settings.themes.systemDesc' },
]

export default function AppearanceTab({ themeMode, onThemeChange, showMessage }: Props) {
  const { t } = useTranslation()

  const handleThemeChange = (mode: ThemeMode) => {
    onThemeChange(mode)
    storeTheme(mode)
    applyTheme(mode)
    showMessage(t('settings.themeApplied'))
  }

  return (
    <div className="settings-section">
      <h2>{t('settings.tabs.appearance')}</h2>
      <p className="section-desc">{t('settings.appearanceDesc')}</p>
      <div className={s.themeOptions}>
        {THEME_OPTIONS.map((opt) => (
          <button
            key={opt.value}
            className={themeMode === opt.value ? `${s.themeOption} ${s.themeOptionActive}` : s.themeOption}
            onClick={() => handleThemeChange(opt.value)}
          >
            <div className={s.themePreview} data-preview={opt.value} />
            <div className={s.themeInfo}>
              <span className={s.themeLabel}>{t(opt.labelKey)}</span>
              <span className={s.themeDesc}>{t(opt.descKey)}</span>
            </div>
          </button>
        ))}
      </div>
    </div>
  )
}
