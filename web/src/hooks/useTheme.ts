import { useCallback, useSyncExternalStore } from 'react'

/**
 * Subscribe to the effective theme (the `data-theme` attribute on <html>).
 *
 * Canvas and inline-SVG surfaces (Chart.js, progress rings) cannot inherit
 * CSS classes, so they must re-read their colors when the theme flips. This
 * hook is how they learn that it happened.
 */
export function useTheme(): 'light' | 'dark' {
  const subscribe = useCallback((cb: () => void) => {
    const obs = new MutationObserver(cb)
    obs.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme'],
    })
    return () => obs.disconnect()
  }, [])

  const getSnapshot = useCallback(() => {
    return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light'
  }, [])

  // Server render has no document; treat as light until it hydrates.
  const getServerSnapshot = useCallback(() => 'light' as const, [])

  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}
