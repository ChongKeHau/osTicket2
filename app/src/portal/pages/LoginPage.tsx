import { useId, useRef, useState, type FormEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { requestAccess, requestLink } from '../../api/portal'
import { ApiError } from '../../api/sessionStore'
import { Banner, errorMessage } from '../../ui/Banner'
import { Button } from '../../ui/Button'
import { FormTable } from '../../ui/FormTable'
import { usePortalAuth } from '../PortalAuthContext'
import { CheckEmailPage } from './CheckEmailPage'
import s from './LoginPage.module.css'

/** 401 must not say which half was wrong; anything else (rate limit, network) is shown as-is. */
function signInError(err: unknown): string {
  if (err instanceof ApiError && err.status === 401) return 'Invalid email or password'
  return errorMessage(err)
}

/** Portal sign-in: password or emailed link on the left, guest ticket access on the right. */
export function LoginPage() {
  return (
    <div className={s.grid}>
      <SignInForm />
      <GuestForm />
    </div>
  )
}

function SignInForm() {
  const { login, notice } = usePortalAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const from = (location.state as { from?: string } | null)?.from ?? '/portal/tickets'
  const headingId = useId()
  const emailRef = useRef<HTMLInputElement>(null)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [emailError, setEmailError] = useState<string | undefined>()
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [linkSent, setLinkSent] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setBusy(true); setError(null); setEmailError(undefined)
    try {
      await login(email, password)
      navigate(from, { replace: true })
    } catch (err) {
      setError(signInError(err))
      setBusy(false)
    }
  }

  async function onLink() {
    setError(null)
    if (!email.trim()) {
      setEmailError('Enter your email address')
      emailRef.current?.focus()
      return
    }
    setEmailError(undefined)
    setBusy(true)
    try {
      await requestLink(email.trim())
      setLinkSent(true)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section aria-labelledby={linkSent ? undefined : headingId} className={s.column}>
      {linkSent ? (
        <CheckEmailPage heading="Check your email" body="If that address has an account, we sent a sign-in link" />
      ) : (
        <form onSubmit={onSubmit}>
          <h2 id={headingId}>Sign in</h2>
          {notice && !error && <Banner level="info">{notice}</Banner>}
          {error && <Banner level="error">{error}</Banner>}
          <FormTable sections={[{ title: 'Your Account', rows: [
            { id: 'login-email', label: 'Email', required: true, error: emailError,
              control: <input id="login-email" ref={emailRef} type="email" required autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} /> },
            { id: 'login-password', label: 'Password', required: true,
              control: <input id="login-password" type="password" required autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /> },
          ] }]} />
          <div className={s.actions}>
            <Button type="submit" variant="primary" disabled={busy}>Sign In</Button>
            <Button type="button" disabled={busy} onClick={() => void onLink()}>Email me a sign-in link</Button>
          </div>
          <p className={s.links}>
            <Link to="/portal/register">Create an account</Link>
            <Link to="/portal/reset">Forgot password</Link>
          </p>
        </form>
      )}
    </section>
  )
}

function GuestForm() {
  const headingId = useId()
  const [email, setEmail] = useState('')
  const [number, setNumber] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [sent, setSent] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setBusy(true); setError(null)
    try {
      await requestAccess(email.trim(), number.trim())
      setSent(true)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section aria-labelledby={sent ? undefined : headingId} className={s.column}>
      {sent ? (
        <CheckEmailPage heading="Check your email" body="If the ticket and email match, we sent an access link" />
      ) : (
        <form onSubmit={onSubmit}>
          <h2 id={headingId}>Check a ticket as a guest</h2>
          {error && <Banner level="error">{error}</Banner>}
          <FormTable sections={[{ title: 'Ticket Access', rows: [
            { id: 'guest-email', label: 'Email', required: true,
              control: <input id="guest-email" type="email" required autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} /> },
            { id: 'guest-number', label: 'Ticket Number', required: true,
              control: <input id="guest-number" required value={number} onChange={(e) => setNumber(e.target.value)} /> },
          ] }]} />
          <div className={s.actions}>
            <Button type="submit" variant="primary" disabled={busy}>Email me an access link</Button>
          </div>
        </form>
      )}
    </section>
  )
}
