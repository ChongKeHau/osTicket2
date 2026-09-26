import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { LoadingScreen } from '../components/LoadingScreen'
import { Banner, errorMessage } from '../ui/Banner'
import { useAuth } from './AuthContext'

export function RequireAuth() {
  const { status, error, retry } = useAuth()
  const location = useLocation()
  if (status === 'loading') return <LoadingScreen />
  if (status === 'error') return (
    <div style={{ padding: 24 }}>
      <Banner level="error">
        {errorMessage(error)}{' '}
        <button type="button" onClick={retry}>Retry</button>
      </Banner>
    </div>
  )
  if (status === 'anonymous') return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />
  return <Outlet />
}
