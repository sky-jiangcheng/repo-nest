import { useTranslation } from 'react-i18next'
import { getToday, getYesterday } from '../utils/dates'
import s from './DatePicker.module.css'

interface Props {
  value: string
  onChange: (date: string) => void
}

function DatePicker({ value, onChange }: Props) {
  const { t } = useTranslation()
  const today = getToday()
  const yesterday = getYesterday()

  return (
    <div className={s.datePicker} role="group" aria-label={t('common.date', { defaultValue: 'Date' })}>
      <button
        className={`btn btn-sm seg-chip ${value === yesterday ? 'seg-chip-active' : ''}`}
        aria-pressed={value === yesterday}
        onClick={() => onChange(yesterday)}
      >
        {t('common.yesterday')}
      </button>
      <button
        className={`btn btn-sm seg-chip ${value === today ? 'seg-chip-active' : ''}`}
        aria-pressed={value === today}
        onClick={() => onChange(today)}
      >
        {t('common.today')}
      </button>
      <input
        type="date"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label={t('common.date', { defaultValue: 'Date' })}
        className={`form-input date-input ${s.dateInput}`}
      />
    </div>
  )
}

export default DatePicker
