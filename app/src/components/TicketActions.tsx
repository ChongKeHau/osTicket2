import type { Ticket } from '../api/types'
import { assignableStaff, useReferenceData } from '../hooks/useReferenceData'
import { useTicketMutations } from '../hooks/useTicketMutations'
import { staffName } from '../lib/format'
import { useBanner } from '../ui/BannerContext'
import { errorMessage } from '../ui/Banner'
import { Button } from '../ui/Button'
import { Menu, type MenuItem } from '../ui/Menu'

export function TicketActions({ ticket, onCompose }: { ticket: Ticket; onCompose: (tab: 'reply' | 'note') => void }) {
  const { staff, departments, statuses } = useReferenceData()
  const m = useTicketMutations(ticket.id)
  const { flash } = useBanner()
  const run = (message: string, p: Promise<unknown>) => { p.then(() => flash('notice', message), (e: unknown) => flash('error', errorMessage(e))) }

  const assignItems: MenuItem[] = [
    ...assignableStaff(staff, ticket.department.id).map((s) => ({
      label: staffName(s), disabled: ticket.assignee?.id === s.id,
      onSelect: () => run(`Ticket assigned to ${staffName(s)}`, m.assign.mutateAsync(s.id)),
    })),
    { label: 'Unassign', danger: true, disabled: !ticket.assignee, onSelect: () => run('Ticket unassigned', m.assign.mutateAsync(null)) },
  ]
  const transferItems: MenuItem[] = departments.map((d) => ({
    label: d.name, disabled: d.id === ticket.department.id,
    onSelect: () => run(`Ticket transferred to ${d.name}`, m.transfer.mutateAsync(d.id)),
  }))
  const statusItems: MenuItem[] = statuses.map((st) => ({
    label: st.name, disabled: st.id === ticket.status.id,
    onSelect: () => run(`Status set to ${st.name}`, m.setStatus.mutateAsync(st.id)),
  }))

  return (
    <>
      <Button variant="primary" onClick={() => onCompose('reply')}>Post Reply</Button>
      <Button onClick={() => onCompose('note')}>Post Note</Button>
      <Menu label={ticket.assignee ? 'Reassign' : 'Assign'} items={assignItems} />
      <Menu label="Transfer" items={transferItems} />
      <Menu label={`Status: ${ticket.status.name}`} items={statusItems} />
    </>
  )
}
