import { Link } from 'react-router-dom'
import type { Ticket } from '../api/types'
import { formatDateTime } from '../lib/format'
import { StatusBadge } from './StatusBadge'

export function TicketRow({ ticket: t }: { ticket: Ticket }) {
  return (
    <tr>
      <td><Link to={`/tickets/${t.id}`}>{t.number}</Link></td>
      <td><Link to={`/tickets/${t.id}`}>{t.subject}</Link></td>
      <td>{t.requester_name || t.requester_email}</td>
      <td>{t.department.name}</td>
      <td><StatusBadge state={t.state} name={t.status.name} /></td>
      <td>{t.priority.name}</td>
      <td>{t.assignee?.name ?? <span className="muted">Unassigned</span>}</td>
      <td>{formatDateTime(t.last_message_at)}</td>
    </tr>
  )
}
