import { useMutation } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { register } from '../../api/portal'
import { ApiError } from '../../api/sessionStore'
import { splitErrors } from '../../lib/forms'
import { Banner, errorMessage } from '../../ui/Banner'
import { FormActions } from '../../ui/FormActions'
import { FormTable } from '../../ui/FormTable'
import { CheckEmailPage } from './CheckEmailPage'

const KNOWN = ['name', 'email']

/** Name and email only: the emailed confirm link lands on the profile page to set a password. */
export function RegisterPage() {
  const [form, setForm] = useState({ name: '', email: '' })
  const m = useMutation({ mutationFn: register })

  if (m.isSuccess) {
    return (
      <CheckEmailPage heading="Check your email" body="Check your email to confirm your account"
        linkTo="/portal/login" linkLabel="Back to sign in" />
    )
  }

  const conflict = m.error instanceof ApiError && m.error.status === 409
  const { fields, banner } = splitErrors(conflict ? null : m.error, KNOWN)

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    m.mutate({ email: form.email.trim(), name: form.name.trim() })
  }

  return (
    <form onSubmit={onSubmit}>
      <h2>Create an Account</h2>
      {conflict && (
        <Banner level="error">
          An account with that email already exists. <Link to="/portal/login">Sign in</Link>
        </Banner>
      )}
      {banner ? <Banner level="error">{errorMessage(banner)}</Banner> : null}
      <FormTable sections={[{ title: 'Your Information', rows: [
        { id: 'name', label: 'Name', required: true, error: fields.name,
          control: <input id="name" required autoComplete="name" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} /> },
        { id: 'email', label: 'Email', required: true, error: fields.email,
          control: <input id="email" type="email" required autoComplete="email" value={form.email} onChange={(e) => setForm((f) => ({ ...f, email: e.target.value }))} /> },
      ] }]} />
      <FormActions saving={m.isPending} cancelTo="/portal/login" saveLabel="Create Account" />
    </form>
  )
}
