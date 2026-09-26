import type { ReactNode } from 'react'
import s from './Badge.module.css'

// Relative luminance (WCAG) of a #rgb / #rrggbb colour, or null for anything else (e.g. a var()).
function luminance(color: string): number | null {
  const raw = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(color.trim())?.[1]
  if (!raw) return null
  const hex = raw.length === 3 ? raw.replace(/./g, (c) => c + c) : raw
  const channel = (i: number) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * channel(0) + 0.7152 * channel(2) + 0.0722 * channel(4)
}

export function Badge({ color, fg, children }: { color?: string; fg?: string; children: ReactNode }) {
  if (!color) return <span className={s.badge}>{children}</span>
  const light = (luminance(color) ?? 0) > 0.6
  const style = {
    backgroundColor: color,
    color: fg ?? (light ? 'var(--text)' : 'var(--on-accent)'),
    // A pale fill (e.g. #FFFFFF) would vanish against the page; outline it instead.
    borderColor: light ? 'var(--border-strong)' : color,
  }
  return <span className={s.badge} style={style}>{children}</span>
}
