'use client'

import { ReactNode, useSyncExternalStore } from 'react'
const subscribe = () => () => {}

/** True only after the component has mounted on the client. Use to gate browser-only values. */
export function useMounted(): boolean {
  return useSyncExternalStore(subscribe, () => true, () => false)
}

/**
 * Renders children only after client mount, so SSR output stays deterministic.
 * Use for anything that differs between server and client (window/localStorage reads,
 * user timezone/locale values, random ids, live clocks, third-party widgets).
 * Pass a `fallback` matching the final size to avoid layout shift.
 */
export function ClientOnly({ children, fallback = null }: { children: ReactNode; fallback?: ReactNode }) {
  const mounted = useMounted()
  return <>{mounted ? children : fallback}</>
}
