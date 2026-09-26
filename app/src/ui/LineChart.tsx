import s from './LineChart.module.css'

export interface Series { label: string; points: number[]; color: string }

const W = 800, H = 260, PAD = { l: 40, r: 12, t: 12, b: 28 }

function shortDate(iso: string): string {
  const d = new Date(iso + 'T00:00:00Z')
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' })
}

function niceMax(v: number): number {
  if (v <= 5) return Math.max(1, Math.ceil(v))
  const p = 10 ** Math.floor(Math.log10(v))
  const n = Math.ceil(v / p)
  return (n <= 2 ? 2 : n <= 5 ? 5 : 10) * p
}

export function LineChart({ series, labels, height = H }: { series: Series[]; labels: string[]; height?: number }) {
  const n = labels.length
  const max = niceMax(Math.max(0, ...series.flatMap((sr) => sr.points)))
  const x = (i: number) => (n <= 1 ? PAD.l : PAD.l + (i * (W - PAD.l - PAD.r)) / (n - 1))
  const y = (v: number) => height - PAD.b - (v / max) * (height - PAD.t - PAD.b)
  // Small counts get one tick per whole ticket instead of 1.3 / 2.5 / 3.8.
  const ticks = max <= 5 ? Array.from({ length: max + 1 }, (_, i) => i) : [0, max / 4, max / 2, (3 * max) / 4, max]
  // The end labels sit on the plot edges; anchor them inward so they stay inside the viewBox.
  const anchor = (i: number) => (i === 0 ? 'start' : i === n - 1 ? 'end' : 'middle')
  // At most 8 x labels: every `every`-th index (≤ 7 steps after 0) plus the last one.
  const every = Math.max(1, Math.ceil((n - 1) / 7))
  const name = series.map((sr) => sr.label).join(', ')
  return (
    <figure className={s.fig}>
      <svg viewBox={`0 0 ${W} ${height}`} role="img" aria-label={`Line chart of ${name}`} className={s.svg}>
        {ticks.map((t) => (
          <g key={t}>
            <line x1={PAD.l} x2={W - PAD.r} y1={y(t)} y2={y(t)} className={s.grid} />
            <text x={PAD.l - 6} y={y(t) + 4} textAnchor="end" data-axis="y" className={s.tick}>{Number.isInteger(t) ? t : t.toFixed(1)}</text>
          </g>
        ))}
        {labels.map((l, i) => (i % every === 0 || i === n - 1) && <text key={l} x={x(i)} y={height - 8} textAnchor={anchor(i)} data-axis="x" className={s.tick}>{shortDate(l)}</text>)}
        {series.map((sr) => (
          <g key={sr.label} stroke={sr.color} fill={sr.color}>
            <path data-series={sr.label} d={sr.points.map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ')} fill="none" strokeWidth={2} />
            {sr.points.map((v, i) => <circle key={i} data-point cx={x(i)} cy={y(v)} r={3}><title>{`${sr.label}, ${labels[i] ? shortDate(labels[i]) : i + 1}: ${v}`}</title></circle>)}
          </g>
        ))}
      </svg>
      <figcaption><ul className={s.legend}>{series.map((sr) => <li key={sr.label}><span className={s.swatch} style={{ background: sr.color }} />{sr.label}</li>)}</ul></figcaption>
    </figure>
  )
}
