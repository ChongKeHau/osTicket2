import { useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { Button, type ButtonVariant } from './Button'
import s from './Menu.module.css'

export interface MenuItem { label: string; onSelect: () => void; danger?: boolean; disabled?: boolean }

export function Menu({ label, items, variant = 'default', align = 'right' }: { label: ReactNode; items: MenuItem[]; variant?: ButtonVariant; align?: 'left' | 'right' }) {
  const [open, setOpen] = useState(false)
  const [cursor, setCursor] = useState(-1)
  const root = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const id = useId()

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => { if (!root.current?.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [open])

  const close = (restore: boolean) => { setOpen(false); setCursor(-1); if (restore) trigger.current?.focus() }
  const enabled = items.map((it, i) => (it.disabled ? -1 : i)).filter((i) => i >= 0)
  const move = (dir: 1 | -1) => {
    if (enabled.length === 0) return
    const pos = enabled.indexOf(cursor)
    const next = pos === -1 ? (dir === 1 ? enabled[0] : enabled[enabled.length - 1]) : enabled[(pos + dir + enabled.length) % enabled.length]
    setCursor(next ?? -1)
  }
  const select = (i: number) => { const it = items[i]; if (!it || it.disabled) return; close(true); it.onSelect() }
  const onKey = (e: KeyboardEvent) => {
    if (!open) { if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setOpen(true); setCursor(enabled[0] ?? -1) } return }
    if (e.key === 'Escape') { e.preventDefault(); close(true) }
    else if (e.key === 'ArrowDown') { e.preventDefault(); move(1) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); move(-1) }
    else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); select(cursor) }
    else if (e.key === 'Tab') close(false)
  }

  return (
    <div className={s.root} ref={root} onKeyDown={onKey}>
      <Button ref={trigger} variant={variant} aria-haspopup="menu" aria-expanded={open} aria-controls={id} onClick={() => (open ? close(false) : setOpen(true))}>{label} <span aria-hidden="true">▾</span></Button>
      {open && (
        <ul role="menu" id={id} className={`${s.menu} ${align === 'left' ? s.left : s.right}`}>
          {items.map((it, i) => (
            <li key={it.label} role="menuitem" aria-disabled={it.disabled || undefined} tabIndex={-1}
              className={[s.item, it.danger && s.danger, i === cursor && s.cursor, it.disabled && s.disabled].filter(Boolean).join(' ')}
              onMouseEnter={() => !it.disabled && setCursor(i)} onClick={() => select(i)}>{it.label}</li>
          ))}
        </ul>
      )}
    </div>
  )
}
