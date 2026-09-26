import { useMutation } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { requestReset } from '../../api/portal'
import { Banner, errorMessage } from '../../ui/Banner'
import { FormActions } from '../../ui/FormActions'
import { FormTable } from '../../ui/FormTable'
import { rateLimitMessage } from '../rateLimit'
import { CheckEmailPage } from './CheckEmailPage'

/** Requests a password-reset link; the API answers 202 whether or not the address exists. */
export function ResetPage() {
  const [email, setEmail] = useState('')
  const m = useMutation({ mutationFn: requestReset })

  if (m.isSuccess) {
    return (
      <CheckEmailPage heading="Check your email" body="If that address has an account, we sent a password reset link"
        linkTo="/portal/login" linkLabel="Back to sign in" />
    )
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    m.mutate(email.trim())
  }

  return (
    <form onSubmit={onSubmit}>
      <h2>Reset Your Password</h2>
      {m.error ? <Banner level="error">{rateLimitMessage(m.error) ?? errorMessage(m.error)}</Banner> : null}
      <FormTable sections={[{ title: 'Your Account', rows: [
        { id: 'email', label: 'Email', required: true,
          control: <input id="email" type="email" required autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} /> },
      ] }]} />
      <FormActions saving={m.isPending} cancelTo="/portal/login" saveLabel="Email me a reset link" />
    </form>
  )
}
