import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { getTicket } from '../api/tickets'
import { ErrorBanner } from '../components/ErrorBanner'
import { LoadingScreen } from '../components/LoadingScreen'
import { TicketHeader } from '../components/TicketHeader'
import { ticketQueryKey } from '../hooks/useTicketMutations'

export function TicketDetailPage() {
  const params = useParams()
  const id = Number(params.id)
  const q = useQuery({ queryKey: ticketQueryKey(id), queryFn: () => getTicket(id), enabled: Number.isInteger(id) && id > 0 })

  if (!Number.isInteger(id) || id <= 0) return <NotFound />
  if (q.isLoading) return <LoadingScreen />
  if (q.error instanceof ApiError && q.error.status === 404) return <NotFound />
  if (q.error) return <ErrorBanner error={q.error} onRetry={() => void q.refetch()} />
  if (!q.data) return null

  return (
    <div>
      <p><Link to="/tickets">← Tickets</Link></p>
      <TicketHeader ticket={q.data} />
      <section aria-label="Thread" className="panel" />
      <section aria-label="Composer" className="panel" />
    </div>
  )
}

function NotFound() {
  return (
    <div className="panel">
      <h1>Ticket not found</h1>
      <Link to="/tickets">Back to tickets</Link>
    </div>
  )
}
