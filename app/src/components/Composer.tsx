import { useState, type FormEvent } from 'react'
import { ApiError } from '../api/client'
import { note, reply } from '../api/tickets'
import { useInvalidateTicket } from '../hooks/useTicketMutations'
import { useReferenceData } from '../hooks/useReferenceData'
import { useBanner } from '../ui/BannerContext'
import { errorMessage } from '../ui/Banner'
import { Button } from '../ui/Button'
import { Tabs } from '../ui/Tabs'
import s from './Composer.module.css'
import { FileUpload, type PendingFile } from './FileUpload'

export type ComposerTab = 'reply' | 'note'

const TABS = [{ id: 'reply', label: 'Reply' }, { id: 'note', label: 'Internal Note' }]

interface Props { ticketId: number; tab: ComposerTab; onTab: (t: ComposerTab) => void; requesterEmail: string }

export function Composer({ ticketId, tab, onTab, requesterEmail }: Props) {
  const { statuses } = useReferenceData()
  const { flash } = useBanner()
  const invalidate = useInvalidateTicket(ticketId)
  const [body, setBody] = useState('')
  const [statusId, setStatusId] = useState<number | ''>('')
  const [noteTitle, setNoteTitle] = useState('')
  const [noteBody, setNoteBody] = useState('')
  const [replyFiles, setReplyFiles] = useState<PendingFile[]>([])
  const [noteFiles, setNoteFiles] = useState<PendingFile[]>([])
  const [fields, setFields] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)

  const pending = tab === 'reply' ? replyFiles : noteFiles
  const fileIds = pending.filter((p) => p.status === 'done' && p.fileId !== undefined).map((p) => p.fileId as number)
  const uploading = pending.some((p) => p.status === 'uploading')

  /** Field errors go under their inputs; anything else is flashed. Returns false on failure. */
  async function attempt(post: () => Promise<unknown>): Promise<boolean> {
    setBusy(true); setFields({})
    try { await post(); return true }
    catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length > 0) setFields(err.fields)
      else flash('error', errorMessage(err))
      return false
    } finally { setBusy(false) }
  }

  // A status change rides inside the reply, so the server applies both in one transaction.
  async function postReply(e: FormEvent) {
    e.preventDefault()
    const ok = await attempt(() => reply(ticketId, { body, format: 'text', ...(statusId !== '' ? { status_id: statusId } : {}), file_ids: fileIds }))
    if (!ok) return
    flash('notice', statusId !== '' ? 'Reply posted and status updated' : 'Reply posted')
    setBody(''); setStatusId(''); setReplyFiles([])
    await invalidate()
  }

  async function postNote(e: FormEvent) {
    e.preventDefault()
    const ok = await attempt(() => note(ticketId, { ...(noteTitle.trim() ? { title: noteTitle.trim() } : {}), body: noteBody, format: 'text', file_ids: fileIds }))
    if (!ok) return
    setNoteTitle(''); setNoteBody(''); setNoteFiles([])
    flash('notice', 'Note posted')
    await invalidate()
  }

  const fieldError = (k: string) => fields[k] && <span className={s.fieldError}>{fields[k]}</span>
  const attachments = (id: string) => (
    <>
      <span className={s.label}>Attachments</span>
      <div><FileUpload inputId={id} pending={pending} onChange={tab === 'reply' ? setReplyFiles : setNoteFiles} />{fieldError('file_ids')}</div>
    </>
  )

  return (
    <section id="composer" className={s.box} aria-label="Compose">
      <Tabs tabs={TABS} active={tab} onChange={(t) => { setFields({}); onTab(t as ComposerTab) }}>
        {tab === 'reply' ? (
          <form key="reply" onSubmit={(e) => void postReply(e)} className={s.form}>
            <label htmlFor="reply-to">To</label>
            <input id="reply-to" value={requesterEmail} readOnly />
            <label htmlFor="reply-body">Response</label>
            <div><textarea id="reply-body" className={s.body} rows={8} value={body} onChange={(e) => setBody(e.target.value)} required />{fieldError('body')}</div>
            {attachments('reply-files')}
            <label htmlFor="reply-status">Set status to</label>
            <select id="reply-status" value={statusId} onChange={(e) => setStatusId(e.target.value ? Number(e.target.value) : '')}>
              <option value="">— keep current —</option>
              {statuses.map((st) => <option key={st.id} value={st.id}>{st.name}</option>)}
            </select>
            <div className={s.actions}><Button type="submit" variant="primary" disabled={busy || uploading || !body.trim()}>Post Reply</Button></div>
          </form>
        ) : (
          <form key="note" onSubmit={(e) => void postNote(e)} className={s.form}>
            <label htmlFor="note-title">Title</label>
            <div><input id="note-title" value={noteTitle} onChange={(e) => setNoteTitle(e.target.value)} />{fieldError('title')}</div>
            <label htmlFor="note-body">Note</label>
            <div><textarea id="note-body" className={s.body} rows={6} value={noteBody} onChange={(e) => setNoteBody(e.target.value)} required />{fieldError('body')}</div>
            {attachments('note-files')}
            <div className={s.actions}><Button type="submit" disabled={busy || uploading || !noteBody.trim()}>Post Note</Button></div>
          </form>
        )}
      </Tabs>
    </section>
  )
}
