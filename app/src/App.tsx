import { Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import { RequireAdmin } from './auth/RequireAdmin'
import { RequireAuth } from './auth/RequireAuth'
import { Layout } from './components/Layout'
import { LoginPage } from './pages/LoginPage'
import { NewTicketPage } from './pages/NewTicketPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { AdminLayout } from './pages/admin/AdminLayout'
import { DepartmentFormPage } from './pages/admin/DepartmentFormPage'
import { DepartmentListPage } from './pages/admin/DepartmentListPage'
import { TicketDetailPage } from './pages/TicketDetailPage'
import { TicketListPage } from './pages/TicketListPage'

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<RequireAuth />}>
          <Route element={<Layout />}>
            <Route path="/" element={<Navigate to="/tickets" replace />} />
            <Route path="/tickets" element={<TicketListPage />} />
            <Route path="/tickets/new" element={<NewTicketPage />} />
            <Route path="/tickets/:id" element={<TicketDetailPage />} />
            <Route element={<RequireAdmin />}>
              <Route path="/admin" element={<AdminLayout />}>
                <Route index element={<Navigate to="/admin/departments" replace />} />
                <Route path="departments" element={<DepartmentListPage />} />
                <Route path="departments/new" element={<DepartmentFormPage />} />
                <Route path="departments/:id" element={<DepartmentFormPage />} />
              </Route>
            </Route>
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Route>
      </Routes>
    </AuthProvider>
  )
}
