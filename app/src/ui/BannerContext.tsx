import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Banner, type BannerLevel } from './Banner'
import { useLocation } from 'react-router-dom'

interface Flash { level: BannerLevel; message: string }
interface Value { current: Flash | null; flash(level: BannerLevel, message: string): void; clear(): void }

const Ctx = createContext<Value | null>(null)

export function BannerProvider({ children }: { children: ReactNode }) {
  const [current, setCurrent] = useState<Flash | null>(null)
  const ref = useRef<HTMLDivElement>(null)
  const flashedAt = useRef(0)
  const { key } = useLocation()
  const flash = useCallback((level: BannerLevel, message: string) => {
    flashedAt.current = Date.now()
    setCurrent({ level, message })
  }, [])
  const clear = useCallback(() => setCurrent(null), [])
  const value = useMemo(() => ({ current, flash, clear }), [current, flash, clear])
  // The flash renders at the top of <main>; bring it on screen when the page is scrolled (e.g. composer).
  // jsdom has no scrollIntoView, hence the optional call.
  useEffect(() => {
    if (current) ref.current?.scrollIntoView?.({ block: 'nearest' })
  }, [current])
  // A flash belongs to the page it was raised on: clear it on the next navigation, unless that
  // navigation follows within a second (create-then-redirect keeps its "Ticket created" message).
  useEffect(() => {
    if (Date.now() - flashedAt.current > 1000) setCurrent(null)
  }, [key])
  return (
    <Ctx.Provider value={value}>
      {current && <Banner ref={ref} level={current.level} onDismiss={clear}>{current.message}</Banner>}
      {children}
    </Ctx.Provider>
  )
}

export function useBanner(): Value {
  const v = useContext(Ctx)
  if (!v) throw new Error('useBanner outside BannerProvider')
  return v
}
