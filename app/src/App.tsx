import { useQuery } from '@tanstack/react-query'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { listTickets } from './api/tickets'
import { AuthProvider } from './auth/AuthContext'
import { RequireAuth } from './auth/RequireAuth'
import { Layout } from './components/Layout'
import { LoginPage } from './pages/LoginPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { TicketListPage } from './pages/TicketListPage'

function TicketsPlaceholder() {
  // React Router reuses this component instance across sibling routes (/tickets/new,
  // /tickets/:id) since it's the same element type at the same tree position, so a route change
  // alone won't remount it. Fold the pathname into the query key so navigating between these
  // routes still issues a fresh GET /tickets (Tasks 6 and 9 replace this placeholder with real pages).
  const location = useLocation()
  const q = useQuery({
    queryKey: ['tickets', location.pathname, { page: 1, page_size: 25 }],
    queryFn: () => listTickets({ page: 1, page_size: 25 }),
  })
  return <h1>Tickets{q.data ? ` (${q.data.total})` : ''}</h1>
}

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<RequireAuth />}>
          <Route element={<Layout />}>
            <Route path="/" element={<Navigate to="/tickets" replace />} />
            <Route path="/tickets" element={<TicketListPage />} />
            <Route path="/tickets/new" element={<TicketsPlaceholder />} />
            <Route path="/tickets/:id" element={<TicketsPlaceholder />} />
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Route>
      </Routes>
    </AuthProvider>
  )
}
