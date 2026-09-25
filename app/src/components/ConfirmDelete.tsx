import { useState } from 'react'

/** Inline two-step delete: no browser dialogs. The caller surfaces any error from onConfirm. */
export function ConfirmDelete({ label, onConfirm }: { label: string; onConfirm: () => Promise<unknown> }) {
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
    return <button type="button" aria-label={`Delete ${label}`} onClick={() => setConfirming(true)}>Delete</button>
  }
  return (
    <span className="row" style={{ gap: 6 }}>
      <button type="button" disabled={busy} onClick={() => void confirm()} style={{ borderColor: 'var(--danger)', color: 'var(--danger)' }}>Confirm</button>
      <button type="button" disabled={busy} onClick={() => setConfirming(false)}>Cancel</button>
    </span>
  )
}
