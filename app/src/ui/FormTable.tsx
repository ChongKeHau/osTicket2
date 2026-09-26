import { cloneElement, isValidElement, type ReactElement, type ReactNode } from 'react'
import s from './FormTable.module.css'

export interface FormRow { id?: string; label: string; required?: boolean; error?: string; help?: string; control: ReactNode }
export interface FormSection { title: string; rows: FormRow[] }

export function FormTable({ sections }: { sections: FormSection[] }) {
  return (
    <table className={s.table}>
      {sections.map((sec) => (
        <tbody key={sec.title}>
          <tr><th colSpan={2} className={s.section}>{sec.title}</th></tr>
          {sec.rows.map((r) => {
            const control = isValidElement(r.control)
              ? cloneElement(r.control as ReactElement<Record<string, unknown>>, {
                  'aria-invalid': r.error ? true : undefined,
                  'aria-describedby': r.error ? `${r.id}-err` : undefined,
                })
              : r.control
            return (
              <tr key={r.label} className={r.error ? s.hasError : undefined}>
                <td className={s.label}>
                  <label htmlFor={r.id} className={r.required ? s.required : undefined}>{r.label}{r.required && <span className={s.star} aria-hidden="true">*</span>}</label>
                </td>
                <td className={s.control}>
                  <div data-invalid={r.error ? 'true' : undefined}>{control}</div>
                  {r.help && <div className={s.help}>{r.help}</div>}
                  {r.error && <div id={`${r.id}-err`} className={s.error} role="alert">{r.error}</div>}
                </td>
              </tr>
            )
          })}
        </tbody>
      ))}
    </table>
  )
}
