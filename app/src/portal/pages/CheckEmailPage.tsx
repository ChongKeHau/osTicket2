import type { ReactNode } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'

export interface CheckEmailPageProps {
  heading: string
  body: ReactNode
  linkTo?: string
  linkLabel?: string
}

/**
 * Generic "check your email" / confirmation screen: a heading, body text and an optional link.
 * Reused as-is by Task 7's register and reset pages, which pass their own copy.
 */
export function CheckEmailPage({ heading, body, linkTo, linkLabel }: CheckEmailPageProps) {
  return (
    <div>
      <h2>{heading}</h2>
      <p>{body}</p>
      {linkTo && linkLabel && <p><Link to={linkTo}>{linkLabel}</Link></p>}
    </div>
  )
}

interface OpenedState { id?: number; anonymous?: boolean }

/**
 * Route element for `/portal/opened/:number`. Derives CheckEmailPage's props from the
 * navigation state OpenTicketPage sets: an anonymous submitter gets the emailed-link note,
 * a signed-in one gets a link straight to the ticket instead.
 */
export function TicketOpenedPage() {
  const { number } = useParams<{ number: string }>()
  const location = useLocation()
  const state = (location.state as OpenedState | null) ?? {}
  const anonymous = state.anonymous ?? true
  return (
    <CheckEmailPage
      heading={`Ticket #${number} opened`}
      body={anonymous
        ? 'We emailed you a link to follow this ticket.'
        : 'You can view and reply to this ticket from your account.'}
      linkTo={!anonymous && state.id !== undefined ? `/portal/tickets/${state.id}` : undefined}
      linkLabel="View ticket"
    />
  )
}
