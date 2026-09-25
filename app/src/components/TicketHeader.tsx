import { useState, type FormEvent } from 'react'
import { ApiError } from '../api/client'
import type { Ticket, UpdateTicketInput } from '../api/types'
import { useAuth } from '../auth/AuthContext'
import { useReferenceData } from '../hooks/useReferenceData'
import { useTicketMutations } from '../hooks/useTicketMutations'
import { formatDateTime, fromLocalInput, staffName, toLocalInput } from '../lib/format'
import { ErrorBanner } from './ErrorBanner'
import { StatusBadge } from './StatusBadge'
import styles from './TicketHeader.module.css'

function formFromTicket(ticket: Ticket) {
  return {
    subject: ticket.subject, priority_id: ticket.priority.id, topic_id: ticket.topic?.id ?? 0,
    due_at: ticket.due_at ? toLocalInput(ticket.due_at) : '', requester_name: ticket.requester_name, requester_email: ticket.requester_email,
  }
}

export function TicketHeader({ ticket }: { ticket: Ticket }) {
  const { isAdmin, departmentIds } = useAuth()
  const { statuses, departments, staff, priorities, topics } = useReferenceData()
  const m = useTicketMutations(ticket.id)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState(formFromTicket(ticket))
  const [initialDue, setInitialDue] = useState(form.due_at)
  const error = m.setStatus.error ?? m.assign.error ?? m.transfer.error ?? m.update.error
  const fields = error instanceof ApiError ? error.fields : {}
  const busy = m.setStatus.isPending || m.assign.isPending || m.transfer.isPending || m.update.isPending

  const assignable = staff.filter((s) => s.is_active && (s.is_admin || s.department_ids.includes(ticket.department.id)))
  const visibleDepts = departments.filter((d) => isAdmin || departmentIds.includes(d.id))

  function save(e: FormEvent) {
    e.preventDefault()
    const input: UpdateTicketInput = {
      subject: form.subject, priority_id: form.priority_id, topic_id: form.topic_id || null,
      requester_name: form.requester_name, requester_email: form.requester_email,
    }
    // Send due_at only when the agent changed it, so saving other fields never rewrites the due date.
    if (form.due_at !== initialDue) input.due_at = form.due_at ? fromLocalInput(form.due_at) : null
    m.update.mutate(input, { onSuccess: () => setEditing(false) })
  }

  return (
    <section aria-label="Ticket header" className="panel">
      <h1 className={styles.title}>{ticket.number} · {ticket.subject}</h1>
      {error && <ErrorBanner error={error} />}
      <dl className={styles.grid}>
        <dt>Requester</dt><dd>{ticket.requester_name} &lt;{ticket.requester_email}&gt;</dd>
        <dt>Status</dt><dd>
          <StatusBadge state={ticket.state} name={ticket.status.name} />{' '}
          <select aria-label="Status" value={ticket.status.id} disabled={busy} onChange={(e) => m.setStatus.mutate(Number(e.target.value))}>
            {statuses.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
        </dd>
        <dt>Department</dt><dd>
          <span>{ticket.department.name}</span>{' '}
          <select aria-label="Department" value={ticket.department.id} disabled={busy} onChange={(e) => m.transfer.mutate(Number(e.target.value))}>
            {visibleDepts.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </select>
        </dd>
        <dt>Assignee</dt><dd>
          <span>{ticket.assignee?.name ?? 'Unassigned'}</span>{' '}
          <select aria-label="Assignee" value={ticket.assignee?.id ?? ''} disabled={busy} onChange={(e) => m.assign.mutate(e.target.value ? Number(e.target.value) : null)}>
            <option value="">Unassigned</option>
            {assignable.map((s) => <option key={s.id} value={s.id}>{staffName(s)}</option>)}
          </select>
        </dd>
        <dt>Topic</dt><dd>{ticket.topic?.name ?? <span className="muted">None</span>}</dd>
        <dt>Priority</dt><dd>{ticket.priority.name}</dd>
        <dt>Created</dt><dd>{formatDateTime(ticket.created_at)}</dd>
        <dt>Due</dt><dd>{ticket.due_at ? formatDateTime(ticket.due_at) : <span className="muted">None</span>}</dd>
        {ticket.closed_at && <><dt>Closed</dt><dd>{formatDateTime(ticket.closed_at)}</dd></>}
      </dl>
      {!editing && <button type="button" onClick={() => { const f = formFromTicket(ticket); setForm(f); setInitialDue(f.due_at); setEditing(true) }}>Edit</button>}
      {editing && (
        <form onSubmit={save} className={styles.editForm}>
          <div className="field"><label htmlFor="subject">Subject</label>
            <input id="subject" value={form.subject} onChange={(e) => setForm({ ...form, subject: e.target.value })} required />
            {fields.subject && <span className="field-error">{fields.subject}</span>}</div>
          <div className="field"><label htmlFor="priority">Priority</label>
            <select id="priority" value={form.priority_id} onChange={(e) => setForm({ ...form, priority_id: Number(e.target.value) })}>
              {priorities.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></div>
          <div className="field"><label htmlFor="topic">Topic</label>
            <select id="topic" value={form.topic_id} onChange={(e) => setForm({ ...form, topic_id: Number(e.target.value) })}>
              <option value={0}>None</option>{topics.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</select></div>
          <div className="field"><label htmlFor="due">Due</label>
            <input id="due" type="datetime-local" value={form.due_at} onChange={(e) => setForm({ ...form, due_at: e.target.value })} /></div>
          <div className="field"><label htmlFor="rname">Requester name</label>
            <input id="rname" value={form.requester_name} onChange={(e) => setForm({ ...form, requester_name: e.target.value })} /></div>
          <div className="field"><label htmlFor="remail">Requester email</label>
            <input id="remail" type="email" value={form.requester_email} onChange={(e) => setForm({ ...form, requester_email: e.target.value })} required />
            {fields.requester_email && <span className="field-error">{fields.requester_email}</span>}</div>
          <div className="row">
            <button type="submit" className="primary" disabled={busy}>Save</button>
            <button type="button" onClick={() => setEditing(false)}>Cancel</button>
          </div>
        </form>
      )}
    </section>
  )
}
