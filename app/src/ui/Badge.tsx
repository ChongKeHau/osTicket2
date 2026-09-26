import type { ReactNode } from 'react'
import s from './Badge.module.css'

export function Badge({ color, children }: { color?: string; children: ReactNode }) {
  return <span className={s.badge} style={color ? { backgroundColor: color, color: 'var(--on-accent)', borderColor: color } : undefined}>{children}</span>
}
