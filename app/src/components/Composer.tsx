import { useMutation } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { ApiError } from '../api/client'
import { note, reply } from '../api/tickets'
import { useInvalidateTicket } from '../hooks/useTicketMutations'
import { useReferenceData } from '../hooks/useReferenceData'
import styles from './Composer.module.css'
import { ErrorBanner } from './ErrorBanner'
import { FileUpload, type PendingFile } from './FileUpload'

type Tab = 'reply' | 'note'

export function Composer({ ticketId }: { ticketId: number }) {
  const [tab, setTab] = useState<Tab>('reply')
  const [body, setBody] = useState('')
  const [title, setTitle] = useState('')
  const [statusId, setStatusId] = useState<number | ''>('')
  const [pending, setPending] = useState<PendingFile[]>([])
  const { statuses } = useReferenceData()
  const invalidate = useInvalidateTicket(ticketId)

  const fileIds = pending.filter((p) => p.status === 'done' && p.fileId !== undefined).map((p) => p.fileId as number)
  const uploading = pending.some((p) => p.status === 'uploading')

  const m = useMutation({
    mutationFn: () => tab === 'reply'
      ? reply(ticketId, { body, format: 'text', ...(statusId ? { status_id: statusId } : {}), file_ids: fileIds })
      : note(ticketId, { ...(title ? { title } : {}), body, format: 'text', file_ids: fileIds }),
    onSuccess: async () => { setBody(''); setTitle(''); setStatusId(''); setPending([]); await invalidate() },
  })
  const fields = m.error instanceof ApiError ? m.error.fields : {}
  const bannerError = m.error instanceof ApiError && Object.keys(m.error.fields).length > 0 ? null : m.error

  function submit(e: FormEvent) { e.preventDefault(); m.mutate() }

  return (
    <section aria-label="Composer" className="panel">
      <div role="tablist" className={styles.tabs}>
        <button role="tab" type="button" aria-selected={tab === 'reply'} className={tab === 'reply' ? styles.active : ''} onClick={() => setTab('reply')}>Reply</button>
        <button role="tab" type="button" aria-selected={tab === 'note'} className={tab === 'note' ? styles.active : ''} onClick={() => setTab('note')}>Internal note</button>
      </div>
      {bannerError && <ErrorBanner error={bannerError} />}
      <form onSubmit={submit}>
        {tab === 'note' && (
          <div className="field">
            <label htmlFor="note-title">Title</label>
            <input id="note-title" value={title} onChange={(e) => setTitle(e.target.value)} />
            {fields.title && <span className="field-error">{fields.title}</span>}
          </div>
        )}
        <div className="field">
          <label htmlFor="composer-body">{tab === 'reply' ? 'Reply' : 'Note'}</label>
          <textarea id="composer-body" rows={6} value={body} onChange={(e) => setBody(e.target.value)} required />
          {fields.body && <span className="field-error">{fields.body}</span>}
        </div>
        <FileUpload pending={pending} onChange={setPending} inputId="composer-files" />
        {fields.file_ids && <span className="field-error">{fields.file_ids}</span>}
        <div className="row" style={{ marginTop: 12 }}>
          {tab === 'reply' && (
            <label>Set status{' '}
              <select aria-label="Set status" value={statusId} onChange={(e) => setStatusId(e.target.value ? Number(e.target.value) : '')}>
                <option value="">Keep current</option>
                {statuses.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </label>
          )}
          <button type="submit" className="primary" disabled={m.isPending || uploading || !body.trim()}>
            {tab === 'reply' ? 'Send reply' : 'Add note'}
          </button>
        </div>
      </form>
    </section>
  )
}
