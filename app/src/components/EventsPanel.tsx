import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { getEvents } from '../api/tickets'
import { formatDateTime } from '../lib/format'
import { ErrorBanner } from './ErrorBanner'

export function EventsPanel({ ticketId }: { ticketId: number }) {
  const [open, setOpen] = useState(false)
  const q = useQuery({ queryKey: ['events', ticketId], queryFn: () => getEvents(ticketId), enabled: open })
  return (
    <section aria-label="History" className="panel">
      <button type="button" onClick={() => setOpen((o) => !o)}>{open ? 'Hide history' : 'Show history'}</button>
      {open && q.error && <ErrorBanner error={q.error} onRetry={() => void q.refetch()} />}
      {open && q.data && (
        <ul>
          {q.data.map((ev) => (
            <li key={ev.id}>
              <span className="muted">{formatDateTime(ev.created_at)}</span>{' '}
              <strong>{ev.kind.replace('_', ' ')}</strong>
              {ev.staff && <> by {ev.staff.name || `staff #${ev.staff.id}`}</>}
              {Object.keys(ev.data).length > 0 && <span className="muted"> {JSON.stringify(ev.data)}</span>}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
