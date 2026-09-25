import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { ApiError } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { LoadingScreen } from '../components/LoadingScreen'

export function LoginPage() {
  const { status, login, notice } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const from = (location.state as { from?: string } | null)?.from ?? '/tickets'
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [fields, setFields] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)

  if (status === 'loading') return <LoadingScreen />
  if (status === 'authenticated') return <Navigate to="/tickets" replace />

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setBusy(true); setError(null); setFields({})
    try {
      await login(username, password)
      navigate(from, { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) setError('Invalid username or password')
      else if (err instanceof ApiError && err.status === 400) setFields(err.fields)
      else setError(err instanceof Error ? err.message : 'Sign in failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="panel" style={{ maxWidth: 360, margin: '80px auto' }}>
      <h1>Sign in</h1>
      {notice && <p role="status" className="muted">{notice}</p>}
      {error && <p role="alert" className="field-error">{error}</p>}
      <form onSubmit={(e) => void onSubmit(e)}>
        <div className="field">
          <label htmlFor="username">Username</label>
          <input id="username" autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} required />
          {fields.username && <span className="field-error">{fields.username}</span>}
        </div>
        <div className="field">
          <label htmlFor="password">Password</label>
          <input id="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          {fields.password && <span className="field-error">{fields.password}</span>}
        </div>
        <button type="submit" className="primary" disabled={busy}>Sign in</button>
      </form>
    </div>
  )
}
