import { useTranslation } from 'react-i18next'
import { useEffect, useState } from 'react'
import { getStatusBar, type StatusBarData } from '../api/client'
import { getConnectionKind, subscribeConnection } from '../api/client'
import s from './StatusBar.module.css'

export default function StatusBar() {
  const { t } = useTranslation()
  const [data, setData] = useState<StatusBarData | null>(null)
  const [connKind, setConnKind] = useState(getConnectionKind())

  const fetch = () => {
    getStatusBar().then(setData).catch(() => {})
  }

  useEffect(() => {
    fetch()
    const timer = setInterval(fetch, 30000)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    return subscribeConnection(setConnKind)
  }, [])

  // Update current time every second locally
  const [now, setNow] = useState(new Date())
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(timer)
  }, [])

  const currentTime = now.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })

  const statusText = connKind === 'offline'
    ? t('status.offline', { defaultValue: 'Offline' })
    : connKind === 'backend-down'
      ? t('status.backendDown', { defaultValue: 'Backend unavailable' })
      : null

  return (
    <div className={s.statusBar} aria-live="polite">
      <div className={s.statusLeft}>
        <span className={s.statusItem}>
          <span className={`${s.statusDot} ${connKind !== 'ok' ? s.statusDotError : ''}`} />
          {statusText && <span className="status-warn">{statusText}</span>}
          {t('status.time')}{currentTime}
        </span>
      </div>
      <div className={s.statusRight}>
        {data?.last_commit_time ? (
          <>
            <span className={s.statusItem} title={data.last_commit_msg}>
              {t('status.lastCommit')}<strong>{data.last_commit_time}</strong>
            </span>
            <span className={s.statusSeparator}>|</span>
            <span className={s.statusItem}>
              {t('status.project')}<strong>{data.last_commit_repo}</strong>
            </span>
            <span className={s.statusSeparator}>|</span>
            <span className={s.statusItem}>
              {t('status.branch')}<strong>{data.last_commit_branch || 'unknown'}</strong>
            </span>
          </>
        ) : (
          <span className={`${s.statusItem} muted`}>{t('status.noCommits')}</span>
        )}
      </div>
    </div>
  )
}
