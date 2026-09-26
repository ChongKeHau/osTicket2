import { useState } from 'react'
import { downloadFile } from '../api/files'
import type { AttachmentRef } from '../api/types'

function size(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

/** `download` defaults to the staff file route; the portal passes one bound to its ticket. */
export function AttachmentList({ attachments, download = downloadFile }: {
  attachments: AttachmentRef[]; download?: (fileId: number, name: string) => Promise<void>
}) {
  const [error, setError] = useState<string | null>(null)
  if (attachments.length === 0) return null
  return (
    <ul className="row" style={{ listStyle: 'none', padding: 0, margin: '8px 0 0' }}>
      {attachments.map((a) => (
        <li key={a.file_id}>
          <button type="button" onClick={() => download(a.file_id, a.name).catch(() => setError(`Could not download ${a.name}`))}>
            {a.name} <span className="muted">({size(a.size)})</span>
          </button>
        </li>
      ))}
      {error && <li role="alert" className="field-error">{error}</li>}
    </ul>
  )
}
