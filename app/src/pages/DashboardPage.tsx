import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { type FormEvent, useMemo, useState } from 'react'
import { getDashboardStats } from '../api/dashboard'
import type { DashboardRow } from '../api/types'
import { sortBy } from '../lib/sort'
import { useSubNav } from '../ui/AppShell'
import { Banner, errorMessage } from '../ui/Banner'
import { Button } from '../ui/Button'
import { LineChart } from '../ui/LineChart'
import { type Column, ListTable } from '../ui/ListTable'
import { StickyBar } from '../ui/StickyBar'
import { Tabs } from '../ui/Tabs'
import s from './DashboardPage.module.css'

const PERIODS = [7, 14, 30, 90]
const DEFAULT_PERIOD = 30
const METRICS = ['opened', 'assigned', 'closed', 'reopened'] as const
const COLORS = { opened: 'var(--accent)', assigned: 'var(--warning-fg)', closed: 'var(--green)', reopened: 'var(--danger)' }
const TABS = [{ id: 'by_department', label: 'Department' }, { id: 'by_topic', label: 'Help Topic' }, { id: 'by_staff', label: 'Agent' }] as const
type TabId = (typeof TABS)[number]['id']

const title = (k: string) => k.charAt(0).toUpperCase() + k.slice(1)

/** `YYYY-MM-DD` for today − n days in UTC. */
function isoDaysAgo(n: number): string {
  return new Date(Date.now() - n * 86_400_000).toISOString().slice(0, 10)
}

/** Default start for a period: the window [start, start+period) ends with today, like the API default. */
const defaultStart = (period: number) => isoDaysAgo(period - 1)

export function DashboardPage() {
  useSubNav(useMemo(() => [{ label: 'Overview', to: '/dashboard' }], []))
  const [period, setPeriod] = useState(DEFAULT_PERIOD)
  const [start, setStart] = useState(() => defaultStart(DEFAULT_PERIOD))
  // Until the user picks a start or refreshes, the start follows the period so the window ends today.
  const [autoStart, setAutoStart] = useState(true)
  const [applied, setApplied] = useState(() => ({ start: defaultStart(DEFAULT_PERIOD), period: DEFAULT_PERIOD }))
  const stats = useQuery({ queryKey: ['dashboard', applied], queryFn: () => getDashboardStats(applied.start, applied.period), placeholderData: keepPreviousData })
  const [tab, setTab] = useState<TabId>('by_department')
  const [sort, setSort] = useState('name')
  const d = stats.data
  const empty = d && d.series.every((p) => p.opened + p.assigned + p.closed + p.reopened === 0)
  const rows = useMemo(() => (d ? sortBy(d[tab], sort) : []), [d, tab, sort])
  const totals: DashboardRow = rows.reduce((t, r) => ({ ...t, opened: t.opened + r.opened, assigned: t.assigned + r.assigned, closed: t.closed + r.closed, reopened: t.reopened + r.reopened }), { id: null, name: 'Total', opened: 0, assigned: 0, closed: 0, reopened: 0 })
  // Identity, not name: a department could be called "Total".
  const totalClass = (r: DashboardRow) => (r === totals ? 'total' : undefined)
  const columns: Column<DashboardRow>[] = [
    { key: 'name', label: TABS.find((t) => t.id === tab)!.label, sortKey: 'name', render: (r) => r.name, cellClass: totalClass },
    ...METRICS.map((k) => ({ key: k, label: title(k), sortKey: k, align: 'right' as const, render: (r: DashboardRow) => r[k], cellClass: totalClass })),
  ]
  const changePeriod = (p: number) => {
    setPeriod(p)
    if (autoStart) setStart(defaultStart(p))
  }
  const onRefresh = (e: FormEvent) => {
    e.preventDefault()
    setAutoStart(false)
    // An equal `applied` is the same query key, so React Query would not fetch: refetch explicitly.
    if (start === applied.start && period === applied.period) void stats.refetch()
    else setApplied({ start, period })
  }
  return (
    <>
      <StickyBar title="Dashboard" />
      <form className={s.period} onSubmit={onRefresh}>
        <label htmlFor="start">Start</label>
        <input id="start" type="date" value={start} onChange={(e) => { setAutoStart(false); setStart(e.target.value) }} />
        <label htmlFor="period">Period</label>
        <select id="period" value={period} onChange={(e) => changePeriod(Number(e.target.value))}>{PERIODS.map((p) => <option key={p} value={p}>{p} days</option>)}</select>
        <Button type="submit">Refresh</Button>
      </form>
      {stats.error && <Banner level="error">{errorMessage(stats.error)}</Banner>}
      <h2>Ticket Activity</h2>
      {stats.isPending && <p className="muted" role="status">Loading…</p>}
      {d && (empty ? <p className="muted">No activity in this period</p> : (
        <LineChart labels={d.series.map((p) => p.date)} series={METRICS.map((k) => ({ label: title(k), points: d.series.map((p) => p[k]), color: COLORS[k] }))} />
      ))}
      {/* Only with data: before it arrives or after an error, an empty table would claim "no activity". */}
      {d && (
        <>
          <h2 className={s.statsTitle}>Statistics</h2>
          <Tabs tabs={[...TABS]} active={tab} onChange={(t) => { setTab(t as TabId); setSort('name') }}>
            <ListTable columns={columns} rows={rows.length ? [...rows, totals] : []} rowKey={(r) => `${r.id ?? 'none'}-${r.name}`} sort={sort} onSort={setSort} empty="No activity in this period" />
          </Tabs>
        </>
      )}
    </>
  )
}
