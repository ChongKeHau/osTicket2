const fmt = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : fmt.format(d)
}

export function staffName(s: { first_name: string; last_name: string; username: string }): string {
  const full = `${s.first_name} ${s.last_name}`.trim()
  return full || s.username
}

const pad = (n: number) => String(n).padStart(2, '0')

/** ISO instant -> `YYYY-MM-DDTHH:mm` in the browser's local time, for a datetime-local input. */
export function toLocalInput(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** datetime-local value (local wall-clock) -> ISO instant in UTC. */
export function fromLocalInput(value: string): string {
  return new Date(value).toISOString()
}
