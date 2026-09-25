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
import { StaffFormPage } from './pages/admin/StaffFormPage'
import { StaffListPage } from './pages/admin/StaffListPage'
import { TopicFormPage } from './pages/admin/TopicFormPage'
import { TopicListPage } from './pages/admin/TopicListPage'
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
                <Route path="topics" element={<TopicListPage />} />
                <Route path="topics/new" element={<TopicFormPage />} />
                <Route path="topics/:id" element={<TopicFormPage />} />
                <Route path="staff" element={<StaffListPage />} />
                <Route path="staff/new" element={<StaffFormPage />} />
                <Route path="staff/:id" element={<StaffFormPage />} />
              </Route>
            </Route>
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Route>
      </Routes>
    </AuthProvider>
  )
}
