import { useState } from 'react'
import type { Ticket } from '../api/types'
import { assignableStaff, useReferenceData } from '../hooks/useReferenceData'
import { useTicketMutations } from '../hooks/useTicketMutations'
import { formatDateTime, fromLocalInput, relativeTime, staffName, toLocalInput } from '../lib/format'
import { InlineEdit } from '../ui/InlineEdit'
import s from './TicketInfo.module.css'

const idOrEmpty = (v: string) => (v ? Number(v) : '')

/** The two osTicket-style info tables; every editable field is an inline edit whose draft resets on open. */
export function TicketInfo({ ticket }: { ticket: Ticket }) {
  const { priorities, statuses, departments, staff, topics } = useReferenceData()
  const m = useTicketMutations(ticket.id)
  const [subject, setSubject] = useState(ticket.subject)
  const [statusId, setStatusId] = useState(ticket.status.id)
  const [priorityId, setPriorityId] = useState(ticket.priority.id)
  const [deptId, setDeptId] = useState(ticket.department.id)
  const [due, setDue] = useState('')
  const [name, setName] = useState(ticket.requester_name)
  const [email, setEmail] = useState(ticket.requester_email)
  const [assigneeId, setAssigneeId] = useState<number | ''>('')
  const [topicId, setTopicId] = useState<number | ''>('')
  const agents = assignableStaff(staff, ticket.department.id)
  const topicChoices = topics.filter((t) => t.is_active || t.id === ticket.topic?.id)

  return (
    <div className={s.grid}>
      <table className={s.info}>
        <tbody>
          <tr><th>Status</th><td>
            <InlineEdit label="Status" value={ticket.status.name} onOpen={() => setStatusId(ticket.status.id)} onSave={() => m.setStatus.mutateAsync(statusId)}
              editor={() => <select aria-label="Status editor" value={statusId} onChange={(e) => setStatusId(Number(e.target.value))}>{statuses.map((st) => <option key={st.id} value={st.id}>{st.name}</option>)}</select>} />
          </td></tr>
          <tr><th>Priority</th><td>
            <InlineEdit label="Priority" value={ticket.priority.name} onOpen={() => setPriorityId(ticket.priority.id)} onSave={() => m.update.mutateAsync({ priority_id: priorityId })}
              editor={() => <select aria-label="Priority editor" value={priorityId} onChange={(e) => setPriorityId(Number(e.target.value))}>{priorities.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select>} />
          </td></tr>
          <tr><th>Department</th><td>
            <InlineEdit label="Department" value={ticket.department.name} onOpen={() => setDeptId(ticket.department.id)} onSave={() => m.transfer.mutateAsync(deptId)}
              editor={() => <select aria-label="Department editor" value={deptId} onChange={(e) => setDeptId(Number(e.target.value))}>{departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}</select>} />
          </td></tr>
          <tr><th>Created</th><td><span title={ticket.created_at}>{formatDateTime(ticket.created_at)} <span className="muted">({relativeTime(ticket.created_at)})</span></span></td></tr>
          <tr><th>Due Date</th><td>
            <InlineEdit label="Due Date" value={ticket.due_at ? formatDateTime(ticket.due_at) : '—'} onOpen={() => setDue(ticket.due_at ? toLocalInput(ticket.due_at) : '')}
              onSave={() => m.update.mutateAsync({ due_at: due ? fromLocalInput(due) : null })}
              editor={() => <input aria-label="Due Date editor" type="datetime-local" value={due} onChange={(e) => setDue(e.target.value)} />} />
          </td></tr>
          {ticket.closed_at && <tr><th>Closed</th><td>{formatDateTime(ticket.closed_at)}</td></tr>}
        </tbody>
      </table>
      <table className={s.info}>
        <tbody>
          <tr><th>Subject</th><td>
            <InlineEdit label="Subject" value={ticket.subject} onOpen={() => setSubject(ticket.subject)} onSave={() => m.update.mutateAsync({ subject: subject.trim() })}
              editor={() => <input aria-label="Subject editor" className={s.wide} value={subject} onChange={(e) => setSubject(e.target.value)} required />} />
          </td></tr>
          <tr><th>User</th><td>
            <InlineEdit label="User" value={ticket.requester_name || '—'} onOpen={() => setName(ticket.requester_name)} onSave={() => m.update.mutateAsync({ requester_name: name.trim() })}
              editor={() => <input aria-label="User editor" value={name} onChange={(e) => setName(e.target.value)} />} />
          </td></tr>
          <tr><th>Email</th><td>
            <a href={`mailto:${ticket.requester_email}`}>{ticket.requester_email}</a>{' '}
            <InlineEdit label="Email" value={<span className="sr-only">edit</span>} onOpen={() => setEmail(ticket.requester_email)} onSave={() => m.update.mutateAsync({ requester_email: email.trim() })}
              editor={() => <input aria-label="Email editor" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />} />
          </td></tr>
          <tr><th>Source</th><td>{ticket.source}</td></tr>
          <tr><th>Assigned To</th><td>
            <InlineEdit label="Assigned To" value={ticket.assignee?.name ?? '— unassigned —'} onOpen={() => setAssigneeId(ticket.assignee?.id ?? '')}
              onSave={() => m.assign.mutateAsync(assigneeId === '' ? null : assigneeId)}
              editor={() => (
                <select aria-label="Assigned To editor" value={assigneeId} onChange={(e) => setAssigneeId(idOrEmpty(e.target.value))}>
                  <option value="">— unassigned —</option>
                  {agents.map((a) => <option key={a.id} value={a.id}>{staffName(a)}</option>)}
                </select>
              )} />
          </td></tr>
          <tr><th>Help Topic</th><td>
            <InlineEdit label="Help Topic" value={ticket.topic?.name ?? '—'} onOpen={() => setTopicId(ticket.topic?.id ?? '')}
              onSave={() => m.update.mutateAsync({ topic_id: topicId === '' ? null : topicId })}
              editor={() => (
                <select aria-label="Help Topic editor" value={topicId} onChange={(e) => setTopicId(idOrEmpty(e.target.value))}>
                  <option value="">— none —</option>
                  {topicChoices.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
                </select>
              )} />
          </td></tr>
          <tr><th>Last Updated</th><td><span title={ticket.updated_at}>{formatDateTime(ticket.updated_at)}</span></td></tr>
        </tbody>
      </table>
    </div>
  )
}
