import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { listTickets } from '../api/tickets'
import { ErrorBanner } from '../components/ErrorBanner'
import { FilterBar } from '../components/FilterBar'
import { LoadingScreen } from '../components/LoadingScreen'
import { Pagination } from '../components/Pagination'
import { TicketRow } from '../components/TicketRow'
import { useTicketFilters } from '../hooks/useTicketFilters'
import styles from './TicketListPage.module.css'

export function TicketListPage() {
  const { filter, set } = useTicketFilters()
  const q = useQuery({ queryKey: ['tickets', filter], queryFn: () => listTickets(filter), placeholderData: keepPreviousData })

  function toggleSort(key: string) {
    set({ sort: filter.sort === key ? `-${key}` : key })
  }
  const sortLabel = (key: string, label: string) => {
    const dir = filter.sort === key ? ' ↑' : filter.sort === `-${key}` ? ' ↓' : ''
    return <button type="button" onClick={() => toggleSort(key)}>{label}{dir}</button>
  }

  return (
    <div>
      <div className="row" style={{ justifyContent: 'space-between', marginBottom: 12 }}>
        <h1 style={{ margin: 0 }}>Tickets</h1>
        <Link to="/tickets/new"><button type="button" className="primary">New ticket</button></Link>
      </div>
      <FilterBar filter={filter} onChange={set} />
      {q.isLoading && <LoadingScreen />}
      {q.error && <ErrorBanner error={q.error} onRetry={() => void q.refetch()} />}
      {q.data && (
        <>
          <div className={styles.tableWrap}>
            <table>
              <thead>
                <tr>
                  <th>Number</th><th>Subject</th><th>Requester</th><th>Department</th><th>Status</th>
                  <th>{sortLabel('priority', 'Priority')}</th><th>Assignee</th>
                  <th>{sortLabel('created_at', 'Created')}</th><th>{sortLabel('last_message_at', 'Last message')}</th>
                </tr>
              </thead>
              <tbody>
                {q.data.items.length === 0 && <tr><td colSpan={9}className="muted">No tickets match.</td></tr>}
                {q.data.items.map((t) => <TicketRow key={t.id} ticket={t} />)}
              </tbody>
            </table>
          </div>
          <Pagination page={q.data.page} pageSize={q.data.page_size} total={q.data.total} onPage={(p) => set({ page: p })} />
        </>
      )}
    </div>
  )
}
