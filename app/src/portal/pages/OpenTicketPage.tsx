import { useMutation, useQuery } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { getReference, openTicket, uploadPortalFile } from '../../api/portal'
import type { OpenTicketInput, PortalProfile, PortalReference } from '../../api/types'
import { FileUpload, type PendingFile } from '../../components/FileUpload'
import { LoadingScreen } from '../../components/LoadingScreen'
import { splitErrors } from '../../lib/forms'
import { Banner, errorMessage } from '../../ui/Banner'
import { FormActions } from '../../ui/FormActions'
import { FormTable } from '../../ui/FormTable'
import { usePortalAuth } from '../PortalAuthContext'
import { rateLimitMessage } from '../rateLimit'
import s from './OpenTicketPage.module.css'

const KNOWN = ['name', 'email', 'subject', 'message', 'topic_id', 'dept_id', 'file_ids']

/** Loads the reference data and the session before handing off to the form, which needs both settled to seed itself once. */
export function OpenTicketPage() {
  const { status, user } = usePortalAuth()
  const referenceQ = useQuery({ queryKey: ['portal', 'reference'], queryFn: getReference })

  if (status === 'loading' || referenceQ.isLoading) return <LoadingScreen />
  if (referenceQ.error) return (
    <Banner level="error">
      {errorMessage(referenceQ.error)}{' '}
      <button type="button" onClick={() => void referenceQ.refetch()}>Retry</button>
    </Banner>
  )
  const reference = referenceQ.data
  if (!reference) return null
  // key: reseed the form if the signed-in identity changes (e.g. sign-in/out while this page is open).
  return <OpenTicketForm key={user?.id ?? 'anon'} reference={reference} user={user} />
}

function OpenTicketForm({ reference, user }: { reference: PortalReference; user: PortalProfile | null }) {
  const navigate = useNavigate()
  const [form, setForm] = useState({
    name: user?.name ?? '', email: user?.email ?? '', subject: '', message: '', topic_id: 0, dept_id: 0,
  })
  const [pending, setPending] = useState<PendingFile[]>([])
  const uploading = pending.some((p) => p.status === 'uploading')

  const m = useMutation({
    mutationFn: (input: OpenTicketInput) => openTicket(input),
    onSuccess: (res) => navigate(`/portal/opened/${res.number}`, { state: { id: res.id, anonymous: !user } }),
  })
  const { fields, banner } = splitErrors(m.error, KNOWN)
  const rateLimited = rateLimitMessage(m.error)

  function set<K extends keyof typeof form>(key: K, value: (typeof form)[K]) {
    setForm((f) => ({ ...f, [key]: value }))
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    const input: OpenTicketInput = {
      name: form.name, email: form.email, subject: form.subject, message: form.message, format: 'text',
      topic_id: form.topic_id, dept_id: form.dept_id,
      file_ids: pending.filter((p) => p.status === 'done' && p.fileId !== undefined).map((p) => p.fileId as number),
    }
    m.mutate(input)
  }

  const showDept = reference.departments.length > 0

  return (
    <form onSubmit={onSubmit} className={s.form}>
      <h2>Open a New Ticket</h2>
      {rateLimited
        ? <Banner level="error">{rateLimited}</Banner>
        : (banner ? <Banner level="error">{errorMessage(banner)}</Banner> : null)}
      <FormTable sections={[
        { title: 'Your Information', rows: [
          { id: 'name', label: 'Name', error: fields.name, control: <input id="name" value={form.name} onChange={(e) => set('name', e.target.value)} /> },
          { id: 'email', label: 'Email', required: true, error: fields.email,
            control: <input id="email" type="email" required readOnly={!!user} value={form.email} onChange={(e) => set('email', e.target.value)} /> },
        ] },
        { title: 'Ticket Details', rows: [
          { id: 'topic_id', label: 'Help Topic', error: fields.topic_id,
            control: (
              <select id="topic_id" value={form.topic_id} onChange={(e) => set('topic_id', Number(e.target.value))}>
                <option value={0}>— none —</option>
                {reference.topics.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
              </select>
            ) },
          ...(showDept ? [{ id: 'dept_id', label: 'Department', error: fields.dept_id,
            control: (
              <select id="dept_id" value={form.dept_id} onChange={(e) => set('dept_id', Number(e.target.value))}>
                <option value={0}>Choose…</option>
                {reference.departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
              </select>
            ) }] : []),
          { id: 'subject', label: 'Subject', required: true, error: fields.subject,
            control: <input id="subject" required maxLength={255} value={form.subject} onChange={(e) => set('subject', e.target.value)} /> },
          { id: 'message', label: 'Message', required: true, error: fields.message,
            control: <textarea id="message" rows={8} required value={form.message} onChange={(e) => set('message', e.target.value)} /> },
          { label: 'Attachments', error: fields.file_ids, control: <FileUpload inputId="files" pending={pending} onChange={setPending} upload={uploadPortalFile} /> },
        ] },
      ]} />
      <FormActions saving={m.isPending || uploading} cancelTo="/portal" saveLabel="Open Ticket" />
    </form>
  )
}
