import { Link } from 'react-router-dom'
import { Button } from './Button'
import s from './FormActions.module.css'

export function FormActions({ saving, onReset, cancelTo, saveLabel = 'Save Changes' }: { saving: boolean; onReset?: () => void; cancelTo: string; saveLabel?: string }) {
  return (
    <div className={s.row}>
      <Button type="submit" variant="primary" disabled={saving} aria-busy={saving || undefined}>{saveLabel}</Button>
      {onReset && <Button onClick={onReset} disabled={saving}>Reset</Button>}
      <Link to={cancelTo} className={s.cancel}>Cancel</Link>
    </div>
  )
}
