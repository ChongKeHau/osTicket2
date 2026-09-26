import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { exchange } from '../../api/portal'
import { portal } from '../../api/portalClient'
import type { PortalSession } from '../../api/types'
import { LoadingScreen } from '../../components/LoadingScreen'
import { ApiError } from '../../api/sessionStore'
import { Banner, errorMessage } from '../../ui/Banner'
import { useBanner } from '../../ui/BannerContext'
import { LinkButton } from '../../ui/Button'
import { usePortalAuth } from '../PortalAuthContext'

/** Where each emailed-token kind lands once exchanged. */
function destination(s: PortalSession): string {
  switch (s.kind) {
    case 'access': return `/portal/tickets/${s.ticket_id}`
    case 'confirm':
    case 'reset': return '/portal/profile#password'
    default: return '/portal/tickets'
  }
}

/** `/portal/t/:token`: redeems an emailed one-time link and routes by what it was issued for. */
export function TokenPage() {
  const { token } = useParams<{ token: string }>()
  const { adopt } = usePortalAuth()
  const { flash } = useBanner()
  const navigate = useNavigate()
  const [error, setError] = useState<unknown>(null)
  // Tokens are one-time: StrictMode's double effect run must not spend it twice. No cleanup
  // cancels the first run, since the guarded second run would never finish the job.
  const started = useRef(false)

  useEffect(() => {
    if (started.current || !token) return
    started.current = true
    exchange(token).then(
      (session) => {
        // A guest session is scoped to one ticket; without it there is nothing to show, so drop it
        // rather than adopt it (it would leave a signed-in guest who can reach no page).
        if (session.kind === 'access' && session.ticket_id == null) {
          portal.tokens.clear() // exchange() already stored it
          flash('error', 'This link did not include a ticket. Please request a new one.')
          navigate('/portal', { replace: true })
          return
        }
        adopt(session)
        if (session.kind === 'confirm') flash('notice', 'Email confirmed — set your password')
        navigate(destination(session), { replace: true })
      },
      (err: unknown) => setError(err ?? new Error('Something went wrong')),
    )
  }, [token, adopt, flash, navigate])

  if (!error) return <LoadingScreen />
  // 410 is the spent/expired case; anything else (network, 5xx) is reported as it is.
  const expired = error instanceof ApiError && error.status === 410
  return (
    <div>
      <Banner level="error">{expired ? 'This link has expired or was already used' : errorMessage(error)}</Banner>
      <p>
        <LinkButton to="/portal/login" variant="primary">Email me a new sign-in link</LinkButton>{' '}
        <LinkButton to="/portal/open">Open a new ticket</LinkButton>
      </p>
    </div>
  )
}
