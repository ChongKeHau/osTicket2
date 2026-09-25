import { useState } from 'react'

/**
 * Inline two-step delete: no browser dialogs. The caller surfaces any error from onConfirm.
 * `disabled` (e.g. another row's delete is in flight) disables every button.
 */
export function ConfirmDelete({ label, onConfirm, disabled = false }: { label: string; onConfirm: () => Promise<unknown>; disabled?: boolean }) {
  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)

  async function confirm() {
    setBusy(true)
    try { await onConfirm() } catch { /* reported by the caller's mutation state */ } finally {
      setBusy(false)
      setConfirming(false)
    }
  }

  if (!confirming) {
    return <button type="button" aria-label={`Delete ${label}`} disabled={disabled} onClick={() => setConfirming(true)}>Delete</button>
  }
  return (
    <span className="row" style={{ gap: 6 }}>
      <button type="button" aria-label={`Confirm delete ${label}`} disabled={busy || disabled} onClick={() => void confirm()} style={{ borderColor: 'var(--danger)', color: 'var(--danger)' }}>Confirm</button>
      <button type="button" aria-label={`Cancel delete ${label}`} disabled={busy || disabled} onClick={() => setConfirming(false)}>Cancel</button>
    </span>
  )
}
