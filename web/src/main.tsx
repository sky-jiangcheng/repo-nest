import React from 'react'
import ReactDOM from 'react-dom/client'
import './styles/index.css'
import App from './App'
import './i18n'

// 全局错误边界 - 捕获所有渲染错误
class GlobalErrorBoundary extends React.Component<
  { children: React.ReactNode },
  { hasError: boolean; error: Error | null }
> {
  state = { hasError: false, error: null }

  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error('[GlobalErrorBoundary]', error, info.componentStack)
  }

  render() {
    if (this.state.hasError) {
      const errorMsg = this.state.error ? String(this.state.error) : 'Unknown error'
      return (
        <div style={{ padding: 32, color: 'var(--color-error)', background: 'var(--color-surface)', minHeight: '100vh', display: 'flex', flexDirection: 'column', gap: 12 }}>
          <h1>应用加载出错</h1>
          <pre style={{ background: 'var(--color-muted)', padding: 16, borderRadius: 8 }}>{errorMsg}</pre>
          <button onClick={() => window.location.reload()} style={{ padding: '8px 16px', background: 'var(--color-primary)', color: 'white', border: 'none', borderRadius: 6, cursor: 'pointer' }}>
            重试
          </button>
        </div>
      )
    }
    return this.props.children
  }
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <GlobalErrorBoundary>
      <App />
    </GlobalErrorBoundary>
  </React.StrictMode>,
)


