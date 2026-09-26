import { useState } from 'react'

/**
 * Inline two-step delete: no browser dialogs. The caller surfaces any error from onConfirm.
 * `disabled` (e.g. another row's delete is in flight) disables every button.
 * `startConfirming` skips the first step when the caller already asked (e.g. from a menu);
 * `onCancel` then lets the caller put its own trigger back.
 */
export function ConfirmDelete({ label, onConfirm, disabled = false, startConfirming = false, onCancel }: {
  label: string; onConfirm: () => Promise<unknown>; disabled?: boolean; startConfirming?: boolean; onCancel?: () => void
}) {
  const [confirming, setConfirming] = useState(startConfirming)
  const [busy, setBusy] = useState(false)

  async function confirm() {
    setBusy(true)
    try { await onConfirm() } catch { /* reported by the caller's mutation state */ } finally {
      setBusy(false)
      setConfirming(false)
    }
  }

  function cancel() {
    setConfirming(false)
    onCancel?.()
  }

  if (!confirming) {
    return <button type="button" aria-label={`Delete ${label}`} disabled={disabled} onClick={() => setConfirming(true)}>Delete</button>
  }
  return (
    <span className="row" style={{ gap: 6, justifyContent: 'flex-end' }}>
      <button type="button" aria-label={`Confirm delete ${label}`} disabled={busy || disabled} onClick={() => void confirm()} style={{ borderColor: 'var(--danger)', color: 'var(--danger)' }}>Confirm</button>
      <button type="button" aria-label={`Cancel delete ${label}`} disabled={busy || disabled} onClick={cancel}>Cancel</button>
    </span>
  )
}
