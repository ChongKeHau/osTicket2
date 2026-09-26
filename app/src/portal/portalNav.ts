/** Top tabs for the customer portal; `end` makes Home match only `/portal` itself. */
export const PORTAL_TABS: { label: string; to: string; end?: boolean }[] = [
  { label: 'Support Center Home', to: '/portal', end: true },
  { label: 'Open a New Ticket', to: '/portal/open' },
  { label: 'Check Ticket Status', to: '/portal/login' },
]

/** Replaces "Check Ticket Status" once an account (not a guest) is signed in. */
export const PORTAL_MY_TICKETS_TAB = { label: 'My Tickets', to: '/portal/tickets' }
