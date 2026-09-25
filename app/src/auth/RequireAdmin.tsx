import { Outlet } from 'react-router-dom'
import { NotFoundPage } from '../pages/NotFoundPage'
import { useAuth } from './AuthContext'

/** Renders the admin routes for admins and the not-found page for everyone else. Sits under RequireAuth. */
export function RequireAdmin() {
  const { isAdmin } = useAuth()
  return isAdmin ? <Outlet /> : <NotFoundPage />
}
