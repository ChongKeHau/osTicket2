import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { getEvents } from '../api/tickets'
import { formatDateTime, relativeTime } from '../lib/format'
import { Banner, errorMessage } from '../ui/Banner'
import s from './EventsPanel.module.css'

/** Ticket history, collapsed by default; events are fetched the first time it opens. */
export function EventsPanel({ ticketId }: { ticketId: number }) {
  const [open, setOpen] = useState(false)
  const q = useQuery({ queryKey: ['events', ticketId], queryFn: () => getEvents(ticketId), enabled: open })
  return (
    <details className={s.panel} aria-label="History" onToggle={(e) => setOpen(e.currentTarget.open)}>
      <summary className={s.summary}>History</summary>
      {open && q.error && <Banner level="error">{errorMessage(q.error)}</Banner>}
      {open && q.isLoading && <p className="muted">Loading…</p>}
      {open && q.data && (
        q.data.length === 0 ? <p className="muted">No history yet.</p> : (
          <ul className={s.list}>
            {q.data.map((ev) => (
              <li key={ev.id} className={s.row}>
                <span className={s.kind}>{ev.kind.replaceAll('_', ' ')}</span>
                <span>{ev.staff ? (ev.staff.name || `staff #${ev.staff.id}`) : 'system'}</span>
                <time className={s.time} dateTime={ev.created_at} title={formatDateTime(ev.created_at)}>{relativeTime(ev.created_at)}</time>
              </li>
            ))}
          </ul>
        )
      )}
    </details>
  )
}
