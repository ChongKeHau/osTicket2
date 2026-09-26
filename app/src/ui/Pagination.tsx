import s from './Pagination.module.css'

interface Props { page: number; pageSize: number; total: number; onPage: (p: number) => void }

function pagesToShow(page: number, last: number): number[] {
  const set = new Set<number>([1, last, page - 1, page, page + 1])
  return [...set].filter((p) => p >= 1 && p <= last).sort((a, b) => a - b)
}

export function Pagination({ page, pageSize, total, onPage }: Props) {
  const last = Math.max(1, Math.ceil(total / pageSize))
  if (total <= pageSize) return null
  const from = (page - 1) * pageSize + 1
  const to = Math.min(total, page * pageSize)
  const pages = pagesToShow(page, last)
  return (
    <nav className={s.nav} aria-label="Pagination">
      <span className={s.range}>Showing {from}–{to} of {total}</span>
      <button type="button" className={s.btn} disabled={page <= 1} onClick={() => onPage(page - 1)}>Previous</button>
      {pages.map((p, i) => (
        <span key={p}>
          {i > 0 && pages[i - 1] !== p - 1 && <span className={s.gap}>…</span>}
          <button type="button" className={`${s.btn} ${p === page ? s.active : ''}`} aria-label={`Page ${p}`} aria-current={p === page ? 'page' : undefined} onClick={() => onPage(p)}>{p}</button>
        </span>
      ))}
      <button type="button" className={s.btn} disabled={page >= last} onClick={() => onPage(page + 1)}>Next</button>
    </nav>
  )
}
