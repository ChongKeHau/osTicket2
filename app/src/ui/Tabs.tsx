import type { ReactNode } from 'react'
import s from './Tabs.module.css'

export function Tabs({ tabs, active, onChange, children }: { tabs: { id: string; label: string }[]; active: string; onChange: (id: string) => void; children: ReactNode }) {
  return (
    <div>
      <div role="tablist" className={s.list}>
        {tabs.map((t) => (
          <button key={t.id} role="tab" type="button" id={`tab-${t.id}`} aria-selected={t.id === active} aria-controls={`panel-${t.id}`}
            className={t.id === active ? `${s.tab} ${s.active}` : s.tab} onClick={() => onChange(t.id)}>{t.label}</button>
        ))}
      </div>
      <div role="tabpanel" id={`panel-${active}`} aria-labelledby={`tab-${active}`} className={s.panel}>{children}</div>
    </div>
  )
}
