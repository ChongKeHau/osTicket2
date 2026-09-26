import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { LoadingScreen } from '../components/LoadingScreen'
import { Banner, errorMessage } from '../ui/Banner'
import { usePortalAuth } from './PortalAuthContext'

/** Any portal session (account or guest); anonymous visitors go to the portal sign-in page. */
export function RequirePortalUser() {
  const { status, error, retry } = usePortalAuth()
  const location = useLocation()
  if (status === 'loading') return <LoadingScreen />
  if (status === 'error') return (
    <Banner level="error">
      {errorMessage(error)}{' '}
      <button type="button" onClick={retry}>Retry</button>
    </Banner>
  )
  if (status === 'anonymous') return <Navigate to="/portal/login" replace state={{ from: location.pathname + location.search }} />
  return <Outlet />
}
