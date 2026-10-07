// Toast context: pushToast lives in App (it owns the auto-dismiss timer), but
// the components that need to confirm a side effect — copying a path, saving
// a note from the AI answer — sit several levels down and have no route back
// to App's props. Threading a callback through every intermediate component
// for "the user clicked copy" is exactly the kind of prop drilling this
// avoids; the alternative (a local "Copied!" label that only shows on hover,
// which is what the repo rows used to do) is feedback the user never sees.
import { createContext, useContext } from 'react'
import type { ToastItem } from '../components/Toast'

export type PushToast = (item: Omit<ToastItem, 'id'>) => void

// The App wires the real implementation in. Components that render outside a
// provider (unit tests, isolated stories) get a no-op rather than a crash.
export const ToastContext = createContext<PushToast>(() => {})

export function useToast(): PushToast {
  return useContext(ToastContext)
}
