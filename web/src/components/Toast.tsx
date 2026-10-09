// ToastHost renders transient toast notifications (e.g. knowledge import
// results pushed from the Go runtime via Wails events). Positioned top-right
// so it does not obstruct the navbar or main content.
//
// Auto-dismiss timers are owned solely by the party that pushes a toast
// (App's pushToast, keyed per toast id) — rendering here is pure.
import { useTranslation } from 'react-i18next'
import s from './Toast.module.css'

export interface ToastItem {
  id: string
  kind: 'success' | 'error' | 'info'
  title: string
  message?: string
  duration?: number
  actionLabel?: string
  onAction?: () => void | Promise<void>
}

function ToastHost({
  toasts,
  onDismiss,
}: {
  toasts: ToastItem[]
  onDismiss: (id: string) => void
}) {
  const { t } = useTranslation()
  const kindClass: Record<ToastItem['kind'], string> = {
    success: s.toastSuccess,
    error: s.toastError,
    info: s.toastInfo,
  }

  return (
    <div className={s.toastHost} role="status" aria-live="polite">
      {toasts.map((toast) => (
        <div key={toast.id} className={`${s.toast} ${kindClass[toast.kind]}`}>
          <div className={s.toastBody}>
            <div className={s.toastTitle}>{toast.title}</div>
            {toast.message && <div className={s.toastMessage}>{toast.message}</div>}
            {toast.actionLabel && (
              <button
                type="button"
                className={s.toastAction}
                onClick={async () => {
                  try { await toast.onAction?.() } finally { onDismiss(toast.id) }
                }}
              >
                {toast.actionLabel}
              </button>
            )}
          </div>
          <button className={s.toastClose} onClick={() => onDismiss(toast.id)} aria-label={t('common.close')}>
            ×
          </button>
        </div>
      ))}
    </div>
  )
}

export default ToastHost
