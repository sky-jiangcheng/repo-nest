import { lazy, Suspense, type ReactNode, useEffect, useRef, useState } from 'react'
import { BrowserRouter, HashRouter, Routes, Route, Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import CommandPalette from './components/CommandPalette'
import ToastHost, { type ToastItem } from './components/Toast'
import { listenImportCompleted } from './api/transport'
import { applyTheme, getStoredTheme, listenSystemTheme } from './utils/theme'
import ErrorBoundary from './components/ErrorBoundary'
import { setLanguage, getCurrentLanguage } from './i18n'
import { getConnectionKind, subscribeConnection, startHealthPoll } from './api/transport'
import Icon from './components/Icon'
import BrandMark from './components/BrandMark'

// Lazy-loaded pages. The initial route (Knowledge) is still eagerly loaded by
// the browser, but subsequent navigations only fetch the chunks that are needed.
const Dashboard = lazy(() => import('./pages/Dashboard'))
const ProjectDetail = lazy(() => import('./pages/ProjectDetail'))
const Settings = lazy(() => import('./pages/Settings'))
const Knowledge = lazy(() => import('./pages/Knowledge'))
const NotFound = lazy(() => import('./pages/NotFound'))

type LanguageOption = 'zh-CN' | 'en'

const LANG_OPTIONS: { code: LanguageOption; label: string }[] = [
  { code: 'zh-CN', label: '中文' },
  { code: 'en', label: 'English' },
]

// Minimal page loader shown while a lazy chunk is being fetched.
function PageLoader() {
  return (
    <div className="panel-section">
      <div className="skeleton skeleton-text" style={{ height: 20, width: 180, marginBottom: 12 }} />
      <div className="skeleton skeleton-text" style={{ height: 14, width: '60%', marginBottom: 24 }} />
      <div className="skeleton skeleton-text" style={{ height: 120, width: '100%' }} />
    </div>
  )
}

// ErrorBoundary prevents a render crash in any routed page from black-screening
// the whole app with no way back. Uses the standalone ErrorBoundary component
// with resetKey-based auto-recovery on route changes.

function NavBar({ onOpenPalette }: { onOpenPalette: () => void }) {
  const { pathname } = useLocation()
  const { t } = useTranslation()
  const [langOpen, setLangOpen] = useState(false)
  const currentLang = getCurrentLanguage()
  const langRef = useRef<HTMLDivElement>(null)

  // Close the language dropdown on outside clicks.
  useEffect(() => {
    if (!langOpen) return
    const handler = (e: MouseEvent) => {
      if (langRef.current && !langRef.current.contains(e.target as Node)) setLangOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [langOpen])

  return (
    <header>
      <nav className="navbar" aria-label={t('nav.search', { defaultValue: 'main navigation' })}>
        <div className="nav-left">
          <Link to="/" className="nav-brand">
            {/* The real brand mark, not a text glyph — see BrandMark. 26px:
                the mark is a compact nest-and-card silhouette, and below
                ~24px the bowl's inner arc closes up and it reads as a blob. */}
            <BrandMark size={26} className="nav-brand-mark" />
            <span className="nav-brand-name">RepoNest</span>
          </Link>
          <div className="nav-links">
            <Link to="/" className={navClass(pathname === '/' || pathname === '/knowledge')}>
              {t('nav.knowledge')}
            </Link>
            <Link to="/dashboard" className={navClass(pathname === '/dashboard' || pathname.startsWith('/project'))}>
              {t('nav.dashboard')}
            </Link>
            <Link to="/settings" className={navClass(pathname === '/settings')}>
              {t('nav.settings')}
            </Link>
          </div>
        </div>
        <div className="nav-right">
          <div className="lang-switcher" ref={langRef}>
            <button
              className="nav-lang-btn"
              onClick={() => setLangOpen(v => !v)}
              aria-label={t('nav.searchLabel', { defaultValue: 'Language switch' })}
              aria-expanded={langOpen}
              title={t('nav.searchLabel', { defaultValue: 'Switch language' })}
            >
              <Icon name="globe" size={14} />
              <span className="lang-label">{currentLang === 'zh-CN' ? '中文' : 'EN'}</span>
            </button>
            {langOpen && (
              <div className="lang-dropdown">
                {LANG_OPTIONS.map(opt => (
                  <button
                    key={opt.code}
                    className={`lang-option ${opt.code === currentLang ? 'active' : ''}`}
                    onClick={() => { setLanguage(opt.code); setLangOpen(false) }}
                  >
                    <span>{opt.label}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
          <div role="search">
            <button
              className="nav-palette-btn"
              onClick={onOpenPalette}
              aria-label={t('nav.searchLabel', { defaultValue: 'Open search (⌘K)' })}
              title={t('nav.searchLabel', { defaultValue: 'Open search (⌘K)' })}
              aria-haspopup="dialog"
            >
              <Icon name="search" size={14} />
              <span>{t('nav.search')}</span>
              <kbd className="nav-kbd">⌘K</kbd>
            </button>
          </div>
        </div>
      </nav>
    </header>
  )
}

function navClass(active: boolean) {
  return active ? 'active' : ''
}

function AppRouter({ children }: { children: ReactNode }) {
  // Wails WebViews can reject BrowserRouter's history/location mutations on
  // custom origins. Keep normal browser URLs when served in a browser, but
  // use hash URLs in the desktop shell so navigation never throws a
  // DOMException.
  const wails = typeof window !== 'undefined' && !!(window as unknown as { go?: { app?: { App?: unknown } } }).go?.app?.App
  const Router = wails ? HashRouter : BrowserRouter
  return <Router>{children}</Router>
}

function App() {
  const { t } = useTranslation()
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const [connKind, setConnKind] = useState(getConnectionKind())

  // Start periodic health polling on mount.
  useEffect(() => {
    return startHealthPoll()
  }, [])

  // React to connection state changes.
  useEffect(() => {
    return subscribeConnection(setConnKind)
  }, [])

  // Focus main without touching location (the active router owns navigation).
  const skipToContent = (e: React.MouseEvent<HTMLAnchorElement>) => {
    e.preventDefault()
    const main = document.getElementById('main-content')
    main?.focus()
    main?.scrollIntoView({ block: 'start' })
  }

  // pushToast owns the per-toast auto-dismiss timer (the only scheduling
  // site — ToastHost renders purely).
  const pushToast = (item: Omit<ToastItem, 'id'>) => {
    const id = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
    setToasts(prev => [...prev, { ...item, id }])
    setTimeout(() => setToasts(prev => prev.filter(x => x.id !== id)), item.duration ?? 5000)
  }

  useEffect(() => {
    const mode = getStoredTheme()
    applyTheme(mode)
    return listenSystemTheme(() => {
      if (getStoredTheme() === 'system') applyTheme('system')
    })
  }, [])

  useEffect(() => {
    // Surface knowledge import results as toasts (issue #36). The listener
    // is really unsubscribed now that transport returns the cancel function.
    const unsubscribe = listenImportCompleted((data) => {
      if (data.error) {
        pushToast({ kind: 'error', title: `${t('common.importFailed', { defaultValue: 'Import failed' })} ${data.source}`, message: data.error })
        return
      }
      if (data.created === 0 && data.updated === 0 && data.skipped === 0) {
        return
      }
      pushToast({
        kind: 'success',
        title: `${t('common.imported', { defaultValue: 'Imported' })} ${data.source}`,
        message: `${t('common.add', { defaultValue: '+' })}${data.created} · ${t('common.edit', { defaultValue: '~' })}${data.updated} · ${t('common.remove', { defaultValue: '-' })}${data.skipped}`,
      })
    })
    return unsubscribe
  }, [t])

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setPaletteOpen(o => !o)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  const connBanner = connKind === 'offline'
    ? t('status.offline', { defaultValue: 'Offline' })
    : connKind === 'backend-down'
      ? t('status.backendDown', { defaultValue: 'Backend unavailable' })
      : null

  return (
    <AppRouter>
      <div className="app">
        {connBanner && (
          <div className="conn-banner" role="alert" aria-live="polite">{connBanner}</div>
        )}
        <a className="skip-link" href="#main-content" onClick={skipToContent}>{t('common.show', { defaultValue: 'Skip to main content' })}</a>
        <NavBar onOpenPalette={() => setPaletteOpen(true)} />
        <main id="main-content" className="main-content" tabIndex={-1}>
          <RoutedApp />
        </main>
        <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} />
        <ToastHost toasts={toasts} onDismiss={(id) => setToasts(prev => prev.filter(x => x.id !== id))} />
      </div>
    </AppRouter>
  )
}

// RoutedApp wraps the routes in an ErrorBoundary keyed to the current path, so a
// crash in a destination page (e.g. clicking a scan result) shows a recoverable
// error instead of a black screen, and recovers when the route changes.
// Suspense is placed OUTSIDE the ErrorBoundary so a failed chunk load is also
// caught and displayed as a recoverable error rather than a blank page.
function RoutedApp() {
  const { pathname } = useLocation()
  return (
    <ErrorBoundary resetKey={pathname}>
      <Suspense fallback={<PageLoader />}>
        <Routes>
          <Route path="/" element={<Knowledge />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/project/:id" element={<ProjectDetail />} />
          <Route path="/knowledge" element={<Knowledge />} />
          <Route path="/settings" element={<Settings />} />
          {/* Catch-all: without it an unmatched URL renders nothing at all —
              a blank page below the navbar that reads like a crash. */}
          <Route path="*" element={<NotFound />} />
        </Routes>
      </Suspense>
    </ErrorBoundary>
  )
}

export default App
