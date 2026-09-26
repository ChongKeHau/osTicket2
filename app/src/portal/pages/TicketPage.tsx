import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { ApiError } from '../../api/client'
import { closeTicket, getMyTicket, reopenTicket, replyTicket, uploadPortalFile } from '../../api/portal'
import type { PortalTicket } from '../../api/types'
import { FileUpload, type PendingFile } from '../../components/FileUpload'
import { LoadingScreen } from '../../components/LoadingScreen'
import { formatDateTime, relativeTime } from '../../lib/format'
import { parseId } from '../../lib/forms'
import { Banner, errorMessage } from '../../ui/Banner'
import { useBanner } from '../../ui/BannerContext'
import { Button } from '../../ui/Button'
import { PortalThread } from './PortalThread'
import s from './TicketPage.module.css'

const ticketKey = (id: number) => ['portal', 'ticket', id] as const

/** Refreshes this ticket and every cached list page after a change. */
function useInvalidate(id: number) {
  const qc = useQueryClient()
  return () => Promise.all([
    qc.invalidateQueries({ queryKey: ticketKey(id) }),
    qc.invalidateQueries({ queryKey: ['portal', 'tickets'] }),
  ])
}

export function TicketPage() {
  const id = parseId(useParams().id)
  const ticket = useQuery({ queryKey: ticketKey(id ?? 0), queryFn: () => getMyTicket(id!), enabled: id !== null })
  const t = ticket.data

  if (id === null || (ticket.error instanceof ApiError && ticket.error.status === 404)) return <Banner level="error">Ticket not found.</Banner>
  if (ticket.error) return <Banner level="error">{errorMessage(ticket.error)}</Banner>
  if (!t) return <LoadingScreen />
  return (
    <>
      <div className={s.titleRow}>
        <h2 className={s.title}>{t.subject}</h2>
        <StatusControl key={t.state} ticket={t} />
      </div>
      <table className={s.info} aria-label="Ticket information">
        <tbody>
          <tr><th scope="row">Number</th><td className={s.number}>{t.number}</td></tr>
          <tr><th scope="row">Status</th><td>{t.status.name}</td></tr>
          <tr><th scope="row">Department</th><td>{t.department}</td></tr>
          <tr><th scope="row">Help Topic</th><td>{t.topic ?? '—'}</td></tr>
          <tr><th scope="row">Created</th><td><span title={relativeTime(t.created_at)}>{formatDateTime(t.created_at)}</span></td></tr>
          <tr><th scope="row">Last Updated</th><td><span title={relativeTime(t.updated_at)}>{formatDateTime(t.updated_at)}</span></td></tr>
        </tbody>
      </table>
      <PortalThread ticketId={t.id} entries={t.entries} />
      <ReplyForm key={t.id} ticket={t} />
    </>
  )
}

/** Close (two clicks: "Close ticket", then "Confirm") or Reopen, by the ticket's state. */
function StatusControl({ ticket }: { ticket: PortalTicket }) {
  const qc = useQueryClient()
  const invalidate = useInvalidate(ticket.id)
  const { flash } = useBanner()
  const [confirming, setConfirming] = useState(false)
  const closed = ticket.state === 'closed'
  const change = useMutation({
    mutationFn: () => (closed ? reopenTicket(ticket.id) : closeTicket(ticket.id)),
    onSuccess: async (updated) => {
      qc.setQueryData(ticketKey(ticket.id), updated)
      flash('notice', closed ? 'Ticket reopened' : 'Ticket closed')
      await invalidate()
    },
    onError: (err) => flash('error', errorMessage(err)),
    onSettled: () => setConfirming(false),
  })

  if (closed) return <Button disabled={change.isPending} onClick={() => change.mutate()}>Reopen</Button>
  if (!confirming) return <Button onClick={() => setConfirming(true)}>Close ticket</Button>
  return (
    <span className="row">
      <Button variant="danger" disabled={change.isPending} onClick={() => change.mutate()}>Confirm</Button>
      <Button disabled={change.isPending} onClick={() => setConfirming(false)}>Cancel</Button>
    </span>
  )
}

function ReplyForm({ ticket }: { ticket: PortalTicket }) {
  const invalidate = useInvalidate(ticket.id)
  const { flash } = useBanner()
  const [body, setBody] = useState('')
  const [files, setFiles] = useState<PendingFile[]>([])
  const [fields, setFields] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)
  const fileIds = files.filter((f) => f.status === 'done' && f.fileId !== undefined).map((f) => f.fileId as number)
  const uploading = files.some((f) => f.status === 'uploading')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true); setFields({})
    try {
      await replyTicket(ticket.id, { body, format: 'text', file_ids: fileIds })
    } catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length > 0) setFields(err.fields)
      else flash('error', errorMessage(err))
      return
    } finally { setBusy(false) }
    setBody(''); setFiles([])
    flash('notice', 'Reply posted')
    await invalidate()
  }

  return (
    <section className={s.reply} aria-label="Post a reply">
      <form onSubmit={(e) => void submit(e)} className={s.replyForm}>
        <label htmlFor="portal-reply">Reply</label>
        <div>
          {ticket.state === 'closed' && <p className={s.note}>This ticket is closed. Replying will reopen this ticket.</p>}
          <textarea id="portal-reply" className={s.textarea} rows={6} value={body} onChange={(e) => setBody(e.target.value)} required />
          {fields.body && <span className="field-error">{fields.body}</span>}
        </div>
        <span className={s.label}>Attachments</span>
        <div>
          <FileUpload inputId="portal-reply-files" pending={files} onChange={setFiles} upload={uploadPortalFile} />
          {fields.file_ids && <span className="field-error">{fields.file_ids}</span>}
        </div>
        <div className={s.actions}>
          <Button type="submit" variant="primary" disabled={busy || uploading || !body.trim()}>Post Reply</Button>
        </div>
      </form>
    </section>
  )
}
