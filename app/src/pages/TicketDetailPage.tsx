import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { getTicket } from '../api/tickets'
import { Composer, type ComposerTab } from '../components/Composer'
import { EventsPanel } from '../components/EventsPanel'
import { LoadingScreen } from '../components/LoadingScreen'
import { Thread } from '../components/Thread'
import { TicketActions } from '../components/TicketActions'
import { TicketInfo } from '../components/TicketInfo'
import { ticketQueryKey } from '../hooks/useTicketMutations'
import { parseId } from '../lib/forms'
import { ticketSubNav } from '../nav'
import { useSubNav } from '../ui/AppShell'
import { Banner, errorMessage } from '../ui/Banner'
import { LinkButton } from '../ui/Button'
import { StickyBar } from '../ui/StickyBar'
import s from './TicketDetailPage.module.css'

const newTicket = <LinkButton to="/tickets/new" variant="add">New Ticket</LinkButton>

export function TicketDetailPage() {
  const id = parseId(useParams().id)
  const [params] = useSearchParams()
  useSubNav(useMemo(() => ticketSubNav(params), [params]), newTicket)
  const [tab, setTab] = useState<ComposerTab>('reply')
  const ticket = useQuery({ queryKey: ticketQueryKey(id ?? 0), queryFn: () => getTicket(id!), enabled: id !== null })
  const data = ticket.data
  useEffect(() => { document.title = data ? `#${data.number} – ${data.subject}` : 'Ticket' }, [data])

  const compose = (t: ComposerTab) => {
    setTab(t)
    document.getElementById('composer')?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
  }

  if (id === null || (ticket.error instanceof ApiError && ticket.error.status === 404)) return <Banner level="error">Ticket not found.</Banner>
  if (ticket.error) return <Banner level="error">{errorMessage(ticket.error)}</Banner>
  if (!data) return <LoadingScreen />
  return (
    <>
      <StickyBar title={`Ticket #${data.number}`} actions={<TicketActions ticket={data} onCompose={compose} />} />
      <h3 className={s.subject}>{data.subject}</h3>
      <TicketInfo ticket={data} />
      <Thread ticketId={data.id} />
      <EventsPanel ticketId={data.id} />
      <Composer key={data.id} ticketId={data.id} tab={tab} onTab={setTab} requesterEmail={data.requester_email} />
    </>
  )
}
