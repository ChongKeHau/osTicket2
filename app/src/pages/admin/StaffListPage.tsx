import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { AdminTable } from '../../components/AdminTable'
import { ErrorBanner } from '../../components/ErrorBanner'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import styles from './admin.module.css'

export function StaffListPage() {
  const qc = useQueryClient()
  const { staff, departments, isLoading, error } = useReferenceData()
  const dept = (id: number) => departments.find((d) => d.id === id)?.name ?? '—'
  return (
    <div>
      <div className={styles.head}>
        <h1 style={{ margin: 0 }}>Staff</h1>
        <Link to="/admin/staff/new"><button type="button" className="primary">New staff member</button></Link>
      </div>
      {isLoading && <LoadingScreen />}
      {error && <ErrorBanner error={error} onRetry={() => void qc.refetchQueries({ queryKey: ['ref'] })} />}
      {!isLoading && !error && (
        <AdminTable
          columns={[
            { header: 'Name', cell: (s) => <><Link to={`/admin/staff/${s.id}`}>{staffName(s)}</Link>{!s.is_active && <span className={styles.badge}>Inactive</span>}</> },
            { header: 'Username', cell: (s) => s.username },
            { header: 'Email', cell: (s) => s.email },
            { header: 'Admin', cell: (s) => (s.is_admin ? 'Yes' : 'No') },
            { header: 'Active', cell: (s) => (s.is_active ? 'Yes' : 'No') },
            { header: 'Primary department', cell: (s) => dept(s.primary_dept_id) },
          ]}
          rows={staff}
          empty="No staff yet."
        />
      )}
    </div>
  )
}
