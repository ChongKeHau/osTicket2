import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { Button } from './Button'
import { errorMessage } from './Banner'
import s from './InlineEdit.module.css'

interface Props { label: string; value: ReactNode; editor: (props: { close: () => void }) => ReactNode; onSave: () => Promise<unknown>; disabled?: boolean }

export function InlineEdit({ label, value, editor, onSave, disabled }: Props) {
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const container = useRef<HTMLDivElement>(null)
  const wasOpen = useRef(false)
  const close = () => { setOpen(false); setError(null) }
  const save = async () => {
    setSaving(true); setError(null)
    try { await onSave(); setOpen(false) }
    catch (e) { setError(errorMessage(e)) }
    finally { setSaving(false) }
  }
  useEffect(() => {
    if (open) container.current?.querySelector<HTMLElement>('input,select,textarea,button')?.focus()
  }, [open])
  useEffect(() => {
    if (wasOpen.current && !open) trigger.current?.focus()
    wasOpen.current = open
  }, [open])
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape') { e.stopPropagation(); e.preventDefault(); close() }
  }
  if (!open) {
    const isString = typeof value === 'string'
    return (
      <button ref={trigger} type="button" className={s.trigger} disabled={disabled}
        aria-label={isString ? `${label}: ${value}` : undefined} onClick={() => setOpen(true)}>
        {!isString && <span className="sr-only">{label}:</span>}
        {value} <span aria-hidden="true" className={s.pencil}>✎</span>
      </button>
    )
  }
  return (
    <div className={s.editor} ref={container} onKeyDown={onKey}>
      {editor({ close })}
      <Button variant="primary" size="sm" disabled={saving} onClick={save}>Save</Button>
      <Button size="sm" disabled={saving} onClick={close}>Cancel</Button>
      {error && <span role="alert" className={s.error}>{error}</span>}
    </div>
  )
}
