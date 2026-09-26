import { Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import { RequireAdmin } from './auth/RequireAdmin'
import { RequireAuth } from './auth/RequireAuth'
import { DepartmentFormPage } from './pages/admin/DepartmentFormPage'
import { DepartmentListPage } from './pages/admin/DepartmentListPage'
import { InboundPage } from './pages/admin/email/InboundPage'
import { OutboxPage } from './pages/admin/email/OutboxPage'
import { TemplateFormPage } from './pages/admin/email/TemplateFormPage'
import { TemplateListPage } from './pages/admin/email/TemplateListPage'
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
import { PortalAuthProvider } from './portal/PortalAuthContext'
import { PortalShell } from './portal/PortalShell'
import { RequirePortalAccount } from './portal/RequirePortalAccount'
import { RequirePortalUser } from './portal/RequirePortalUser'
import { AppShell } from './ui/AppShell'

export default function App() {
  return (
    <Routes>
      <Route path="/portal/*" element={<PortalApp />} />
      <Route path="/*" element={<StaffApp />} />
    </Routes>
  )
}

/** The customer portal. Its own session provider; the staff AuthProvider never wraps it. */
function PortalApp() {
  return (
    <Routes>
      <Route element={<PortalAuthProvider><PortalShell /></PortalAuthProvider>}>
        <Route index element={<h2>Support Center</h2>} />
        <Route path="open" element={<h2>Open a New Ticket</h2>} />
        <Route path="login" element={<h2>Sign In</h2>} />
        <Route path="register" element={<h2>Register</h2>} />
        <Route path="reset" element={<h2>Reset</h2>} />
        <Route path="t/:token" element={<h2>Token</h2>} />
        <Route element={<RequirePortalUser />}>
          <Route path="tickets/:id" element={<h2>Ticket</h2>} />
          <Route element={<RequirePortalAccount />}>
            <Route path="tickets" element={<h2>My Tickets</h2>} />
            <Route path="profile" element={<h2>Profile</h2>} />
          </Route>
        </Route>
        <Route path="*" element={<h2>Page not found</h2>} />
      </Route>
    </Routes>
  )
}

/** The staff control panel (agent and admin), under the staff session. */
function StaffApp() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="login" element={<LoginPage />} />
        <Route element={<RequireAuth />}>
          <Route element={<AppShell panel="agent" />}>
            <Route index element={<Navigate to="/tickets" replace />} />
            <Route path="dashboard" element={<DashboardPage />} />
            <Route path="tickets" element={<TicketListPage />} />
            <Route path="tickets/new" element={<NewTicketPage />} />
            <Route path="tickets/:id" element={<TicketDetailPage />} />
          </Route>
          <Route element={<RequireAdmin />}>
            <Route element={<AppShell panel="admin" />}>
              <Route path="admin" element={<Navigate to="/admin/departments" replace />} />
              <Route path="admin/departments" element={<DepartmentListPage />} />
              <Route path="admin/departments/new" element={<DepartmentFormPage />} />
              <Route path="admin/departments/:id" element={<DepartmentFormPage />} />
              <Route path="admin/topics" element={<TopicListPage />} />
              <Route path="admin/topics/new" element={<TopicFormPage />} />
              <Route path="admin/topics/:id" element={<TopicFormPage />} />
              <Route path="admin/staff" element={<StaffListPage />} />
              <Route path="admin/staff/new" element={<StaffFormPage />} />
              <Route path="admin/staff/:id" element={<StaffFormPage />} />
              <Route path="admin/email" element={<Navigate to="/admin/email/templates" replace />} />
              <Route path="admin/email/templates" element={<TemplateListPage />} />
              <Route path="admin/email/templates/:key" element={<TemplateFormPage />} />
              <Route path="admin/email/outbox" element={<OutboxPage />} />
              <Route path="admin/email/inbound" element={<InboundPage />} />
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
