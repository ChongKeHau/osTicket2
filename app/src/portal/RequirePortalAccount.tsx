import { Navigate, Outlet } from 'react-router-dom'
import { usePortalAuth } from './PortalAuthContext'

/** Account-only pages; a guest session is sent to the one ticket it can see. Nest inside RequirePortalUser. */
export function RequirePortalAccount() {
  const { isGuest, ticketId } = usePortalAuth()
  return isGuest ? <Navigate to={`/portal/tickets/${ticketId}`} replace /> : <Outlet />
}
