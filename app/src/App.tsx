import { Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import { RequireAdmin } from './auth/RequireAdmin'
import { RequireAuth } from './auth/RequireAuth'
import { DepartmentFormPage } from './pages/admin/DepartmentFormPage'
import { DepartmentListPage } from './pages/admin/DepartmentListPage'
import { StaffFormPage } from './pages/admin/StaffFormPage'
import { StaffListPage } from './pages/admin/StaffListPage'
import { TopicFormPage } from './pages/admin/TopicFormPage'
import { TopicListPage } from './pages/admin/TopicListPage'
import { DashboardPage } from './pages/DashboardPage'
import { LoginPage } from './pages/LoginPage'
import { NewTicketPage } from './pages/NewTicketPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { TicketDetailPage } from './pages/TicketDetailPage'
import { TicketListPage } from './pages/TicketListPage'
import { AppShell } from './ui/AppShell'

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<RequireAuth />}>
          <Route element={<AppShell panel="agent" />}>
            <Route path="/" element={<Navigate to="/tickets" replace />} />
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/tickets" element={<TicketListPage />} />
            <Route path="/tickets/new" element={<NewTicketPage />} />
            <Route path="/tickets/:id" element={<TicketDetailPage />} />
          </Route>
          <Route element={<RequireAdmin />}>
            <Route element={<AppShell panel="admin" />}>
              <Route path="/admin" element={<Navigate to="/admin/departments" replace />} />
              <Route path="/admin/departments" element={<DepartmentListPage />} />
              <Route path="/admin/departments/new" element={<DepartmentFormPage />} />
              <Route path="/admin/departments/:id" element={<DepartmentFormPage />} />
              <Route path="/admin/topics" element={<TopicListPage />} />
              <Route path="/admin/topics/new" element={<TopicFormPage />} />
              <Route path="/admin/topics/:id" element={<TopicFormPage />} />
              <Route path="/admin/staff" element={<StaffListPage />} />
              <Route path="/admin/staff/new" element={<StaffFormPage />} />
              <Route path="/admin/staff/:id" element={<StaffFormPage />} />
              <Route path="/admin/email" element={<Navigate to="/admin/email/templates" replace />} />
              <Route path="/admin/email/templates" element={<h2>Email Templates</h2>} />
              <Route path="/admin/email/outbox" element={<h2>Outbox</h2>} />
              <Route path="/admin/email/inbound" element={<h2>Inbound Log</h2>} />
            </Route>
          </Route>
          <Route element={<AppShell panel="agent" />}>
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Route>
      </Routes>
    </AuthProvider>
  )
}
