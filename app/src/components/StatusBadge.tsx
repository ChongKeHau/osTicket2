import type { TicketState } from '../api/types'

const colors: Record<TicketState, string> = { open: 'var(--ok)', resolved: 'var(--warn)', closed: 'var(--muted)' }

export function StatusBadge({ state, name }: { state: TicketState; name: string }) {
  return <span style={{ color: colors[state], fontWeight: 600 }}>{name}</span>
}
