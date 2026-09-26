import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useLocation } from 'react-router-dom'
import { getMe, setPassword, updateMe } from '../../api/portal'
import type { PortalProfile } from '../../api/types'
import { splitErrors } from '../../lib/forms'
import { Banner, errorMessage } from '../../ui/Banner'
import { useBanner } from '../../ui/BannerContext'
import { FormActions } from '../../ui/FormActions'
import { FormTable } from '../../ui/FormTable'
import { usePortalAuth } from '../PortalAuthContext'

const ME_KEY = ['portal', 'me'] as const
const TICKETS = '/portal/tickets'

/** The signed-in customer's account: name, email (read-only) and password. A reset session
 *  (no refresh token) lands here from the token page with `has_password: false`. */
export function ProfilePage() {
  const { user } = usePortalAuth()
  const me = useQuery({ queryKey: ME_KEY, queryFn: getMe, initialData: user ?? undefined })
  const profile = me.data
  if (!profile) return null

  return (
    <>
      <h2>Profile</h2>
      {me.error && <Banner level="error">{errorMessage(me.error)}</Banner>}
      <NameForm profile={profile} />
      <PasswordForm hasPassword={profile.has_password} />
    </>
  )
}

function NameForm({ profile }: { profile: PortalProfile }) {
  const { flash, clear } = useBanner()
  const { setUser } = usePortalAuth()
  const qc = useQueryClient()
  const [name, setName] = useState(profile.name)
  const mutation = useMutation({
    mutationFn: () => updateMe({ name }),
    onSuccess: (updated) => {
      qc.setQueryData(ME_KEY, updated)
      setUser(updated)
      flash('notice', 'Profile saved')
    },
  })
  const { fields, banner } = splitErrors(mutation.error, ['name'])

  function submit(e: FormEvent) {
    e.preventDefault()
    clear()
    mutation.mutate()
  }

  return (
    <form onSubmit={submit}>
      {banner ? <Banner level="error">{errorMessage(banner)}</Banner> : null}
      <FormTable sections={[{ title: 'Profile', rows: [
        { id: 'profile-email', label: 'Email',
          control: <input id="profile-email" value={profile.email} disabled /> },
        { id: 'profile-name', label: 'Name', required: true, error: fields.name,
          control: (
            <input id="profile-name" value={name} maxLength={128} aria-required="true"
              onChange={(e) => setName(e.target.value)} />
          ) },
      ] }]} />
      <FormActions saving={mutation.isPending} cancelTo={TICKETS} />
    </form>
  )
}

const KNOWN_WITH_CURRENT = ['password', 'current_password']
const KNOWN_WITHOUT_CURRENT = ['password']

function PasswordForm({ hasPassword }: { hasPassword: boolean }) {
  const { flash, clear } = useBanner()
  const { adopt } = usePortalAuth()
  const qc = useQueryClient()
  const location = useLocation()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [mismatch, setMismatch] = useState(false)
  const sectionRef = useRef<HTMLElement>(null)
  const firstInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (location.hash !== '#password') return
    sectionRef.current?.scrollIntoView?.()
    firstInputRef.current?.focus()
  }, [location.hash])

  const mutation = useMutation({
    mutationFn: () => (hasPassword ? setPassword({ password: next, current_password: current }) : setPassword({ password: next })),
    onSuccess: (session) => {
      // The API ended every session this user held (a reset session included) and answered a
      // fresh full one: adopt it, then seed the profile (has_password is read by this form and
      // the shell) so the next render already has it.
      adopt(session)
      qc.setQueryData(ME_KEY, session.user)
      flash('notice', 'Password updated')
      setCurrent(''); setNext(''); setConfirm('')
    },
  })
  const { fields, banner } = splitErrors(mutation.error, hasPassword ? KNOWN_WITH_CURRENT : KNOWN_WITHOUT_CURRENT)

  function submit(e: FormEvent) {
    e.preventDefault()
    clear()
    setMismatch(false)
    if (next !== confirm) { setMismatch(true); return }
    mutation.mutate()
  }

  const rows = []
  if (hasPassword) {
    rows.push({
      id: 'password-current', label: 'Current Password', required: true, error: fields.current_password,
      control: (
        <input id="password-current" ref={firstInputRef} type="password" aria-required="true"
          value={current} onChange={(e) => setCurrent(e.target.value)} />
      ),
    })
  }
  rows.push({
    id: 'password-new', label: 'New Password', required: true, error: fields.password,
    control: (
      <input id="password-new" ref={hasPassword ? undefined : firstInputRef} type="password" aria-required="true"
        value={next} onChange={(e) => setNext(e.target.value)} />
    ),
  })
  rows.push({
    id: 'password-confirm', label: 'Confirm Password', required: true, error: mismatch ? 'Passwords do not match' : undefined,
    control: (
      <input id="password-confirm" type="password" aria-required="true"
        value={confirm} onChange={(e) => setConfirm(e.target.value)} />
    ),
  })

  return (
    <section ref={sectionRef} aria-label="Password">
      <form onSubmit={submit}>
        {banner ? <Banner level="error">{errorMessage(banner)}</Banner> : null}
        <FormTable sections={[{ title: hasPassword ? 'Password' : 'Set a password', rows }]} />
        <FormActions saving={mutation.isPending} cancelTo={TICKETS} saveLabel="Update Password" />
      </form>
    </section>
  )
}
