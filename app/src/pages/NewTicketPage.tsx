import { useMutation, useQueries, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { ApiError } from '../api/client'
import { listDepartments, listPriorities, listTopics } from '../api/reference'
import { createTicket } from '../api/tickets'
import type { CreateTicketInput } from '../api/types'
import { useAuth } from '../auth/AuthContext'
import { FileUpload, type PendingFile } from '../components/FileUpload'
import { LoadingScreen } from '../components/LoadingScreen'
import { visibleDepartments } from '../hooks/useReferenceData'
import { ticketSubNav } from '../nav'
import { useSubNav } from '../ui/AppShell'
import { Banner, errorMessage } from '../ui/Banner'
import { useBanner } from '../ui/BannerContext'
import { FormActions } from '../ui/FormActions'
import { FormTable } from '../ui/FormTable'
import { StickyBar } from '../ui/StickyBar'

export function NewTicketPage() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { isAdmin, departmentIds } = useAuth()
  const { flash } = useBanner()
  useSubNav(useMemo(() => ticketSubNav(new URLSearchParams()), []), undefined)

  // This page only consumes topics/departments/priorities, so it queries just those three
  // (not the full useReferenceData bundle, which also fetches statuses/staff) — a failure on an
  // endpoint this page never uses (e.g. /staff, /statuses) must not block the form. Same query
  // keys and staleTime as useReferenceData so the cache stays shared with other pages.
  const [topicsQ, departmentsQ, prioritiesQ] = useQueries({
    queries: [
      { queryKey: ['ref', 'topics'], queryFn: listTopics, staleTime: 5 * 60_000 },
      { queryKey: ['ref', 'departments'], queryFn: listDepartments, staleTime: 5 * 60_000 },
      { queryKey: ['ref', 'priorities'], queryFn: listPriorities, staleTime: 5 * 60_000 },
    ],
  })
  const refQueries = [topicsQ, departmentsQ, prioritiesQ]
  const topics = topicsQ.data ?? []
  const departments = departmentsQ.data ?? []
  const visibleDepts = visibleDepartments(departments, { isAdmin, departmentIds })
  const priorities = prioritiesQ.data ?? []
  const isLoading = refQueries.some((q) => q.isLoading)
  const refError = refQueries.find((q) => q.error)?.error ?? null
  const [form, setForm] = useState({ subject: '', message: '', requester_name: '', requester_email: '', topic_id: 0, dept_id: 0, priority_id: 0, source: 'phone', due_at: '' })
  const [pending, setPending] = useState<PendingFile[]>([])
  const uploading = pending.some((p) => p.status === 'uploading')

  const m = useMutation({
    mutationFn: (input: CreateTicketInput) => createTicket(input),
    onSuccess: async (t) => {
      await qc.invalidateQueries({ queryKey: ['tickets'] })
      flash('notice', `Ticket #${t.number} created`)
      navigate(`/tickets/${t.id}`)
    },
  })
  const fields = m.error instanceof ApiError ? m.error.fields : {}
  const banner = m.error instanceof ApiError && Object.keys(m.error.fields).length > 0 ? null : m.error

  function update<K extends keyof typeof form>(key: K, value: (typeof form)[K]) {
    setForm((f) => ({ ...f, [key]: value }))
  }

  function onTopic(id: number) {
    const t = topics.find((x) => x.id === id)
    // Keep the current department when the topic's default is not one this agent may use.
    const dept = t && visibleDepts.some((d) => d.id === t.dept_id) ? t.dept_id : undefined
    setForm((f) => ({ ...f, topic_id: id, dept_id: dept ?? f.dept_id, priority_id: t?.priority_id ?? f.priority_id }))
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    const input: CreateTicketInput = {
      subject: form.subject, message: form.message, message_format: 'text',
      requester_name: form.requester_name, requester_email: form.requester_email,
      ...(form.topic_id ? { topic_id: form.topic_id } : {}),
      ...(form.dept_id ? { dept_id: form.dept_id } : {}),
      ...(form.priority_id ? { priority_id: form.priority_id } : {}),
      source: form.source,
      ...(form.due_at ? { due_at: new Date(form.due_at).toISOString() } : {}),
      file_ids: pending.filter((p) => p.status === 'done' && p.fileId !== undefined).map((p) => p.fileId as number),
    }
    m.mutate(input)
  }

  if (isLoading) return <LoadingScreen />
  if (refError) return (
    <Banner level="error">
      {errorMessage(refError)}{' '}
      <button type="button" onClick={() => { void Promise.all(refQueries.map((q) => q.refetch())) }}>Retry</button>
    </Banner>
  )

  return (
    <form onSubmit={onSubmit}>
      <StickyBar title="New Ticket" />
      {banner && <Banner level="error">{errorMessage(banner)}</Banner>}
      <FormTable sections={[
        { title: 'User Information', rows: [
          { id: 'requester_name', label: 'Name', error: fields.requester_name, control: <input id="requester_name" value={form.requester_name} onChange={(e) => update('requester_name', e.target.value)} /> },
          { id: 'requester_email', label: 'Email', required: true, error: fields.requester_email, control: <input id="requester_email" type="email" required value={form.requester_email} onChange={(e) => update('requester_email', e.target.value)} /> },
        ] },
        { title: 'Ticket Details', rows: [
          { id: 'topic_id', label: 'Help Topic', error: fields.topic_id, control: <select id="topic_id" value={form.topic_id} onChange={(e) => onTopic(Number(e.target.value))}>
            <option value={0}>— none —</option>{topics.filter((t) => t.is_active).map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</select> },
          { id: 'dept_id', label: 'Department', error: fields.dept_id, control: <select id="dept_id" value={form.dept_id} onChange={(e) => update('dept_id', Number(e.target.value))}>
            <option value={0}>Choose…</option>{visibleDepts.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}</select> },
          { id: 'priority_id', label: 'Priority', error: fields.priority_id, control: <select id="priority_id" value={form.priority_id} onChange={(e) => update('priority_id', Number(e.target.value))}>
            <option value={0}>Default</option>{priorities.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select> },
          { id: 'subject', label: 'Subject', required: true, error: fields.subject, control: <input id="subject" required maxLength={255} value={form.subject} onChange={(e) => update('subject', e.target.value)} /> },
          { id: 'message', label: 'Message', required: true, error: fields.message, control: <textarea id="message" rows={8} required value={form.message} onChange={(e) => update('message', e.target.value)} /> },
          { label: 'Attachments', error: fields.file_ids, control: <FileUpload inputId="files" pending={pending} onChange={setPending} /> },
          { id: 'due_at', label: 'Due Date', error: fields.due_at, control: <input id="due_at" type="datetime-local" value={form.due_at} onChange={(e) => update('due_at', e.target.value)} /> },
          { id: 'source', label: 'Source', error: fields.source, control: <select id="source" value={form.source} onChange={(e) => update('source', e.target.value)}>
            <option value="phone">Phone</option><option value="web">Web</option><option value="api">API</option><option value="other">Other</option></select> },
        ] },
      ]} />
      <FormActions saving={m.isPending || uploading} cancelTo="/tickets" saveLabel="Open Ticket" />
    </form>
  )
}
