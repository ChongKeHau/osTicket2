import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { ApiError } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { LoadingScreen } from '../components/LoadingScreen'
import { Banner } from '../ui/Banner'
import { Button } from '../ui/Button'
import s from './LoginPage.module.css'

export function LoginPage() {
  const { status, login, notice } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const from = (location.state as { from?: string } | null)?.from ?? '/tickets'
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [banner, setBanner] = useState<string | null>(null)
  const [fields, setFields] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)

  if (status === 'loading') return <LoadingScreen />
  if (status === 'authenticated') return <Navigate to="/tickets" replace />

  async function submit() {
    setBusy(true); setBanner(null); setFields({})
    try {
      await login(username, password)
      navigate(from, { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) setBanner('Invalid username or password')
      else if (err instanceof ApiError && err.status === 400) setFields(err.fields)
      else setBanner(err instanceof Error ? err.message : 'Sign in failed')
    } finally {
      setBusy(false)
    }
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    void submit()
  }

  return (
    <div className={s.page}>
      <form className={s.card} onSubmit={onSubmit}>
        <h1 className={s.brand}>Ticket Desk</h1>
        <p className="muted">Agent sign in</p>
        {notice && !banner && <Banner level="info">{notice}</Banner>}
        {banner && <Banner level="error">{banner}</Banner>}
        <label className={s.label} htmlFor="username">Username</label>
        <input id="username" name="username" autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} aria-invalid={!!fields.username || undefined} />
        {fields.username && <div className={s.err}>{fields.username}</div>}
        <label className={s.label} htmlFor="password">Password</label>
        <input id="password" name="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
        {fields.password && <div className={s.err}>{fields.password}</div>}
        <Button type="submit" variant="primary" disabled={busy} className={s.submit}>{busy ? 'Signing in…' : 'Sign In'}</Button>
      </form>
    </div>
  )
}
