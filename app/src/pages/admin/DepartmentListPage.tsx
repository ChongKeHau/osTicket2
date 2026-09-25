import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { AdminTable } from '../../components/AdminTable'
import { ConfirmDelete } from '../../components/ConfirmDelete'
import { ErrorBanner } from '../../components/ErrorBanner'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useDepartmentMutations } from '../../hooks/useAdminMutations'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import styles from './admin.module.css'

export function DepartmentListPage() {
  const qc = useQueryClient()
  const { departments, staff, isLoading, error } = useReferenceData()
  const { remove } = useDepartmentMutations()
  const manager = (id: number | null) => {
    const s = staff.find((x) => x.id === id)
    return s ? staffName(s) : '—'
  }
  return (
    <div>
      <div className={styles.head}>
        <h1 style={{ margin: 0 }}>Departments</h1>
        <Link to="/admin/departments/new"><button type="button" className="primary">New department</button></Link>
      </div>
      {isLoading && <LoadingScreen />}
      {error && <ErrorBanner error={error} onRetry={() => void qc.refetchQueries({ queryKey: ['ref'] })} />}
      {remove.error && <ErrorBanner error={remove.error} />}
      {!isLoading && !error && (
        <AdminTable
          columns={[
            { header: 'Name', cell: (d) => <Link to={`/admin/departments/${d.id}`}>{d.name}</Link> },
            { header: 'Public', cell: (d) => (d.is_public ? 'Yes' : 'No') },
            { header: 'Manager', cell: (d) => manager(d.manager_id) },
          ]}
          rows={departments}
          actions={(d) => <ConfirmDelete label={d.name} disabled={remove.isPending} onConfirm={() => remove.mutateAsync(d.id)} />}
          empty="No departments yet."
        />
      )}
    </div>
  )
}
