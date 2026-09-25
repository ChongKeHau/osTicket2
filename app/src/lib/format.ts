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
