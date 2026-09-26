import { useEffect, useRef, type ChangeEvent } from 'react'
import { ApiError } from '../api/client'
import { uploadFile } from '../api/files'
import type { FileInfo } from '../api/types'

/** `token` is the portal upload's access token; the portal sends it back in `file_tokens`. */
export interface PendingFile { key: string; name: string; status: 'uploading' | 'done' | 'error'; fileId?: number; token?: string; error?: string }

/** `upload` defaults to the staff upload; the portal passes `uploadPortalFile`. */
export function FileUpload({ pending, onChange, inputId, upload = uploadFile }: {
  pending: PendingFile[]; onChange: (next: PendingFile[]) => void; inputId: string; upload?: (file: File) => Promise<FileInfo & { token?: string }>
}) {
  const ref = useRef<PendingFile[]>(pending)
  useEffect(() => { ref.current = pending }, [pending])

  function update(key: string, patch: Partial<PendingFile>) {
    if (!ref.current.some((p) => p.key === key)) return
    const next = ref.current.map((p) => (p.key === key ? { ...p, ...patch } : p))
    ref.current = next
    onChange(next)
  }

  function remove(key: string) {
    const next = ref.current.filter((p) => p.key !== key)
    ref.current = next
    onChange(next)
  }

  async function onPick(e: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files ?? [])
    e.target.value = ''
    for (const file of files) {
      const key = `${file.name}-${Date.now()}-${Math.random()}`
      const next = [...ref.current, { key, name: file.name, status: 'uploading' as const }]
      ref.current = next
      onChange(next)
      try {
        const info = await upload(file)
        update(key, { status: 'done', fileId: info.id, ...(info.token ? { token: info.token } : {}) })
      } catch (err) {
        const msg = err instanceof ApiError ? (err.fields.file ?? err.message) : 'upload failed'
        update(key, { status: 'error', error: msg })
      }
    }
  }

  return (
    <div>
      <label htmlFor={inputId}>Attach files</label>{' '}
      <input id={inputId} type="file" multiple onChange={(e) => void onPick(e)} />
      {pending.length > 0 && (
        <ul style={{ margin: '8px 0 0', paddingLeft: 18 }}>
          {pending.map((p) => (
            <li key={p.key}>
              {p.name}{' '}
              {p.status === 'uploading' && <span className="muted">uploading…</span>}
              {p.status === 'error' && <span className="field-error">{p.error}</span>}
              <button type="button" aria-label={`Remove ${p.name}`} onClick={() => remove(p.key)} style={{ marginLeft: 8 }}>×</button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
