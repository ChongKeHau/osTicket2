import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { Banner, type BannerLevel } from './Banner'

interface Flash { level: BannerLevel; message: string }
interface Value { current: Flash | null; flash(level: BannerLevel, message: string): void; clear(): void }

const Ctx = createContext<Value | null>(null)

export function BannerProvider({ children }: { children: ReactNode }) {
  const [current, setCurrent] = useState<Flash | null>(null)
  const flash = useCallback((level: BannerLevel, message: string) => setCurrent({ level, message }), [])
  const clear = useCallback(() => setCurrent(null), [])
  const value = useMemo(() => ({ current, flash, clear }), [current, flash, clear])
  return (
    <Ctx.Provider value={value}>
      {current && <Banner level={current.level} onDismiss={clear}>{current.message}</Banner>}
      {children}
    </Ctx.Provider>
  )
}

export function useBanner(): Value {
  const v = useContext(Ctx)
  if (!v) throw new Error('useBanner outside BannerProvider')
  return v
}
