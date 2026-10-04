// 404 page for any URL that matches no route. Without this the <Routes> tree
// renders nothing at all, so a typo'd or stale URL shows a blank screen below
// the navbar — indistinguishable from a crash.

import { Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Icon from '../components/Icon'

function NotFound() {
  const { t } = useTranslation()
  const { pathname } = useLocation()

  return (
    <div className="notfound">
      <div className="notfound-code" aria-hidden="true">404</div>
      <h1>{t('notFound.title')}</h1>
      <p className="notfound-desc">{t('notFound.desc')}</p>
      <code className="notfound-path">{pathname}</code>
      <div className="notfound-actions">
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
