import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { ErrorBanner } from '../components/ErrorBanner'
import { LoadingScreen } from '../components/LoadingScreen'
import { useAuth } from './AuthContext'

export function RequireAuth() {
  const { status, error, retry } = useAuth()
  const location = useLocation()
  if (status === 'loading') return <LoadingScreen />
  if (status === 'error') return <div style={{ padding: 24 }}><ErrorBanner error={error} onRetry={retry} /></div>
  if (status === 'anonymous') return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />
  return <Outlet />
}
