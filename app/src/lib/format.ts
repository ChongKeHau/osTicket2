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

/** `s` cut to at most `n` characters, ending in an ellipsis when shortened; null/undefined -> ''. */
export function truncate(s: string | null | undefined, n: number): string {
  if (!s) return ''
  return s.length <= n ? s : `${s.slice(0, Math.max(0, n - 1))}…`
}

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

export function relativeTime(iso: string, now: number = Date.now()): string {
  const diff = Math.max(0, now - new Date(iso).getTime())
  const m = Math.floor(diff / 60_000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  if (d < 30) return `${d}d ago`
  return formatDate(iso)
}
