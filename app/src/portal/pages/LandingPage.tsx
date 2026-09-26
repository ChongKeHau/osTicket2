import { LinkButton } from '../../ui/Button'
import { usePortalAuth } from '../PortalAuthContext'
import s from './LandingPage.module.css'

/** `/portal` index: the two entry points every visitor needs, before anything else. */
export function LandingPage() {
  const { status, isGuest } = usePortalAuth()
  const signedIn = status === 'authenticated' && !isGuest
  return (
    <div className={s.hero}>
      <h2>Welcome to the Support Center</h2>
      <p>Open a ticket for any question, or check the status of one you already have.</p>
      <div className={s.actions}>
        <LinkButton to="/portal/open" variant="primary">Open a New Ticket</LinkButton>
        <LinkButton to={signedIn ? '/portal/tickets' : '/portal/login'} variant="default">
          {signedIn ? 'My Tickets' : 'Check Ticket Status'}
        </LinkButton>
      </div>
    </div>
  )
}
