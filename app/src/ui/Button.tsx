import type { ButtonHTMLAttributes, ReactNode, Ref } from 'react'
import { Link } from 'react-router-dom'
import s from './Button.module.css'

export type ButtonVariant = 'default' | 'primary' | 'add' | 'danger'

interface Props extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: 'sm' | 'md'
  ref?: Ref<HTMLButtonElement>
}

export function Button({ variant = 'default', size = 'md', className, type = 'button', ref, ...rest }: Props) {
  return <button ref={ref} type={type} className={[s.btn, s[variant], s[size], className].filter(Boolean).join(' ')} {...rest} />
}

export function LinkButton({ to, variant = 'default', size = 'md', children }: { to: string; variant?: ButtonVariant; size?: 'sm' | 'md'; children: ReactNode }) {
  return <Link to={to} className={[s.btn, s[variant], s[size]].join(' ')}>{children}</Link>
}
