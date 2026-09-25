import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { LoadingScreen } from '../components/LoadingScreen'
import { useAuth } from './AuthContext'

export function RequireAuth() {
  const { status } = useAuth()
  const location = useLocation()
  if (status === 'loading') return <LoadingScreen />
  if (status === 'anonymous') return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />
  return <Outlet />
}
