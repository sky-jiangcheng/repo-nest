// 404 page for any URL that matches no route. Without this the <Routes> tree
// renders nothing at all, so a typo'd or stale URL shows a blank screen below
// the navbar — indistinguishable from a crash.

import { Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Icon from '../components/Icon'
import s from './NotFound.module.css'

function NotFound() {
  const { t } = useTranslation()
  const { pathname } = useLocation()

  return (
    <div className={`${s.notfound}`}>
      <div className={`${s.notfoundCode}`} aria-hidden="true">404</div>
      <h1>{t('notFound.title')}</h1>
      <p className={`${s.notfoundDesc}`}>{t('notFound.desc')}</p>
      <code className={`${s.notfoundPath}`}>{pathname}</code>
      <div className={`${s.notfoundActions}`}>
        <Link to="/" className="btn btn-primary">{t('notFound.backHome')}</Link>
        <Link to="/dashboard" className="btn btn-secondary">
          <Icon name="grid" size={14} />
          {t('nav.dashboard')}
        </Link>
        <Link to="/settings" className="btn btn-secondary">
          <Icon name="settings" size={14} />
          {t('nav.settings')}
        </Link>
      </div>
    </div>
  )
}

export default NotFound
