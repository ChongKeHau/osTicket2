import { useMutation, useQueries, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { ApiError } from '../api/client'
import { listDepartments, listPriorities, listTopics } from '../api/reference'
import { createTicket } from '../api/tickets'
import type { CreateTicketInput } from '../api/types'
import { ErrorBanner } from '../components/ErrorBanner'
import { FileUpload, type PendingFile } from '../components/FileUpload'
import { LoadingScreen } from '../components/LoadingScreen'

const SOURCES = ['web', 'phone', 'api', 'other'] as const

export function NewTicketPage() {
  const navigate = useNavigate()
  const qc = useQueryClient()
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
  const priorities = prioritiesQ.data ?? []
  const isLoading = refQueries.some((q) => q.isLoading)
  const refError = refQueries.find((q) => q.error)?.error ?? null
  const [form, setForm] = useState({ subject: '', message: '', requester_name: '', requester_email: '', topic_id: 0, dept_id: 0, priority_id: 0, source: 'web', due_at: '' })
  const [pending, setPending] = useState<PendingFile[]>([])
  const uploading = pending.some((p) => p.status === 'uploading')

  const m = useMutation({
    mutationFn: (input: CreateTicketInput) => createTicket(input),
    onSuccess: async (t) => { await qc.invalidateQueries({ queryKey: ['tickets'] }); navigate(`/tickets/${t.id}`) },
  })
  const fields = m.error instanceof ApiError ? m.error.fields : {}
  const bannerError = m.error instanceof ApiError && Object.keys(m.error.fields).length > 0 ? null : m.error

  function onTopic(id: number) {
    const t = topics.find((x) => x.id === id)
    setForm((f) => ({ ...f, topic_id: id, dept_id: t?.dept_id ?? f.dept_id, priority_id: t?.priority_id ?? f.priority_id }))
  }

  function submit(e: FormEvent) {
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

  const field = (id: string, label: string, el: ReactNode, err?: string) => (
    <div className="field"><label htmlFor={id}>{label}</label>{el}{err && <span className="field-error">{err}</span>}</div>
  )

  if (isLoading) return <LoadingScreen />
  if (refError) return <ErrorBanner error={refError} onRetry={() => { void Promise.all(refQueries.map((q) => q.refetch())) }} />

  return (
    <div className="panel" style={{ maxWidth: 640 }}>
      <h1>New ticket</h1>
      {bannerError && <ErrorBanner error={bannerError} />}
      <form onSubmit={submit}>
        {field('subject', 'Subject', <input id="subject" value={form.subject} onChange={(e) => setForm({ ...form, subject: e.target.value })} required maxLength={255} />, fields.subject)}
        {field('message', 'Message', <textarea id="message" rows={6} value={form.message} onChange={(e) => setForm({ ...form, message: e.target.value })} required />, fields.message)}
        {field('rname', 'Requester name', <input id="rname" value={form.requester_name} onChange={(e) => setForm({ ...form, requester_name: e.target.value })} />, fields.requester_name)}
        {field('remail', 'Requester email', <input id="remail" type="email" value={form.requester_email} onChange={(e) => setForm({ ...form, requester_email: e.target.value })} required />, fields.requester_email)}
        {field('topic', 'Topic', <select id="topic" value={form.topic_id} onChange={(e) => onTopic(Number(e.target.value))}>
          <option value={0}>None</option>{topics.filter((t) => t.is_active).map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</select>, fields.topic_id)}
        {field('dept', 'Department', <select id="dept" value={form.dept_id} onChange={(e) => setForm({ ...form, dept_id: Number(e.target.value) })}>
          <option value={0}>Choose…</option>{departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}</select>, fields.dept_id)}
        {field('priority', 'Priority', <select id="priority" value={form.priority_id} onChange={(e) => setForm({ ...form, priority_id: Number(e.target.value) })}>
          <option value={0}>Default</option>{priorities.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select>, fields.priority_id)}
        {field('source', 'Source', <select id="source" value={form.source} onChange={(e) => setForm({ ...form, source: e.target.value })}>
          {SOURCES.map((s) => <option key={s} value={s}>{s}</option>)}</select>, fields.source)}
        {field('due', 'Due', <input id="due" type="datetime-local" value={form.due_at} onChange={(e) => setForm({ ...form, due_at: e.target.value })} />, fields.due_at)}
        <FileUpload pending={pending} onChange={setPending} inputId="new-files" />
        {fields.file_ids && <span className="field-error">{fields.file_ids}</span>}
        <div className="row" style={{ marginTop: 12 }}>
          <button type="submit" className="primary" disabled={m.isPending || uploading}>Create ticket</button>
        </div>
      </form>
    </div>
  )
}
