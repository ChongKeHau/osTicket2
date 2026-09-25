import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { AdminTable } from '../../components/AdminTable'
import { ConfirmDelete } from '../../components/ConfirmDelete'
import { ErrorBanner } from '../../components/ErrorBanner'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useTopicMutations } from '../../hooks/useAdminMutations'
import { useReferenceData } from '../../hooks/useReferenceData'
import styles from './admin.module.css'

export function TopicListPage() {
  const qc = useQueryClient()
  const { topics, departments, priorities, isLoading, error } = useReferenceData()
  const { remove } = useTopicMutations()
  const name = (list: { id: number; name: string }[], id: number | null) => list.find((x) => x.id === id)?.name ?? '—'
  return (
    <div>
      <div className={styles.head}>
        <h1 style={{ margin: 0 }}>Topics</h1>
        <Link to="/admin/topics/new"><button type="button" className="primary">New topic</button></Link>
      </div>
      {isLoading && <LoadingScreen />}
      {error && <ErrorBanner error={error} onRetry={() => void qc.refetchQueries({ queryKey: ['ref'] })} />}
      {remove.error && <ErrorBanner error={remove.error} />}
      {!isLoading && !error && (
        <AdminTable
          columns={[
            { header: 'Name', cell: (t) => <Link to={`/admin/topics/${t.id}`}>{t.name}</Link> },
            { header: 'Department', cell: (t) => name(departments, t.dept_id) },
            { header: 'Priority', cell: (t) => name(priorities, t.priority_id) },
            { header: 'Active', cell: (t) => (t.is_active ? 'Yes' : 'No') },
            { header: 'Sort', cell: (t) => String(t.sort_order) },
          ]}
          rows={topics}
          actions={(t) => <ConfirmDelete label={t.name} disabled={remove.isPending} onConfirm={() => remove.mutateAsync(t.id)} />}
          empty="No topics yet."
        />
      )}
    </div>
  )
}
