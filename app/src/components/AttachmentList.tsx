import { useState } from 'react'
import { downloadFile } from '../api/files'
import type { AttachmentRef } from '../api/types'

function size(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export function AttachmentList({ attachments }: { attachments: AttachmentRef[] }) {
  const [error, setError] = useState<string | null>(null)
  if (attachments.length === 0) return null
  return (
    <ul className="row" style={{ listStyle: 'none', padding: 0, margin: '8px 0 0' }}>
      {attachments.map((a) => (
        <li key={a.file_id}>
          <button type="button" onClick={() => downloadFile(a.file_id, a.name).catch(() => setError(`Could not download ${a.name}`))}>
            {a.name} <span className="muted">({size(a.size)})</span>
          </button>
        </li>
      ))}
      {error && <li role="alert" className="field-error">{error}</li>}
    </ul>
  )
}
