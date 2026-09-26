import type { ReactNode } from 'react'
import { ApiError } from '../api/client'
import s from './Banner.module.css'

export type BannerLevel = 'info' | 'notice' | 'warning' | 'error'

const GLYPH: Record<BannerLevel, string> = { info: 'ℹ', notice: '✓', warning: '⚠', error: '✕' }

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  if (err instanceof Error && err.message) return err.message
  return 'Something went wrong'
}

export function Banner({ level, children, onDismiss }: { level: BannerLevel; children: ReactNode; onDismiss?: () => void }) {
  const role = level === 'error' || level === 'warning' ? 'alert' : 'status'
  return (
    <div role={role} className={`${s.banner} ${s[level]}`}>
      <span aria-hidden="true" className={s.glyph}>{GLYPH[level]}</span>
      <div className={s.body}>{children}</div>
      {onDismiss && <button type="button" className={s.close} aria-label="Dismiss" onClick={onDismiss}>×</button>}
    </div>
  )
}
