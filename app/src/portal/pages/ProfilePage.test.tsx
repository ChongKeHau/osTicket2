import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { Route, Routes } from 'react-router-dom'
import { PORTAL_REFRESH_KEY, portal } from '../../api/portalClient'
import { portalFixtures } from '../../test/portal'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'
import { PortalAuthProvider } from '../PortalAuthContext'
import { PortalShell } from '../PortalShell'
import { RequirePortalAccount } from '../RequirePortalAccount'
import { RequirePortalUser } from '../RequirePortalUser'
import { ProfilePage } from './ProfilePage'

const P = '/api/v1/portal'

beforeEach(() => portal.tokens.clear())
afterEach(() => {
  delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView
})

function mount(route = '/portal/profile', refresh = 'prefresh-1') {
  localStorage.setItem(PORTAL_REFRESH_KEY, refresh)
  return renderWithProviders(
    <PortalAuthProvider>
      <Routes>
        <Route path="/portal" element={<PortalShell />}>
          <Route element={<RequirePortalUser />}>
            <Route element={<RequirePortalAccount />}>
              <Route path="tickets" element={<h2>Tickets page</h2>} />
              <Route path="profile" element={<ProfilePage />} />
            </Route>
          </Route>
        </Route>
      </Routes>
    </PortalAuthProvider>,
    { route },
  )
}

it('patches the name and flashes Profile saved', async () => {
  let body: unknown
  server.use(http.patch(`${P}/me`, async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ ...portalFixtures.profile, ...(body as object) })
  }))
  mount()
  const name = await screen.findByLabelText(/Name/)
  expect(name).toHaveValue('Pat Customer')
  await userEvent.clear(name)
  await userEvent.type(name, 'Pat New')
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  expect(await screen.findByText('Profile saved')).toBeInTheDocument()
  expect(body).toEqual({ name: 'Pat New' })
})

it('updates the shell header greeting after a name save', async () => {
  server.use(http.patch(`${P}/me`, async ({ request }) => HttpResponse.json({ ...portalFixtures.profile, ...(await request.json() as object) })))
  mount()
  expect(await screen.findByText('Pat Customer')).toBeInTheDocument()
  const name = await screen.findByLabelText(/Name/)
  await userEvent.clear(name)
  await userEvent.type(name, 'Pat New')
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  expect(await screen.findByText('Profile saved')).toBeInTheDocument()
  expect(screen.getByText('Pat New')).toBeInTheDocument()
  expect(screen.queryByText('Pat Customer')).not.toBeInTheDocument()
})

it('shows a field error when the name update fails', async () => {
  server.use(http.patch(`${P}/me`, () => HttpResponse.json(
    { error: { code: 'invalid', message: 'invalid', fields: { name: 'must not be blank' } } }, { status: 422 },
  )))
  mount()
  await screen.findByLabelText(/Name/)
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  expect(await screen.findByText('must not be blank')).toBeInTheDocument()
})

it('has_password: true shows Current, New and Confirm and posts password + current_password', async () => {
  let body: unknown
  server.use(http.post(`${P}/me/password`, async ({ request }) => {
    body = await request.json()
    return new HttpResponse(null, { status: 204 })
  }))
  mount()
  await screen.findByLabelText(/Name/)
  expect(screen.getByText('Password')).toBeInTheDocument()
  await userEvent.type(screen.getByLabelText(/Current Password/), 'oldpass1')
  await userEvent.type(screen.getByLabelText(/New Password/), 'newpass1')
  await userEvent.type(screen.getByLabelText(/Confirm Password/), 'newpass1')
  await userEvent.click(screen.getByRole('button', { name: 'Update Password' }))
  expect(await screen.findByText('Password updated')).toBeInTheDocument()
  expect(body).toEqual({ password: 'newpass1', current_password: 'oldpass1' })
})

it('has_password: false shows "Set a password" with no Current Password field and posts only password', async () => {
  server.use(http.get(`${P}/me`, () => HttpResponse.json({ ...portalFixtures.profile, has_password: false })))
  let body: unknown
  server.use(http.post(`${P}/me/password`, async ({ request }) => {
    body = await request.json()
    return new HttpResponse(null, { status: 204 })
  }))
  mount()
  await screen.findByLabelText(/Name/)
  expect(await screen.findByText('Set a password')).toBeInTheDocument()
  expect(screen.queryByLabelText(/Current Password/)).not.toBeInTheDocument()
  await userEvent.type(screen.getByLabelText(/New Password/), 'newpass1')
  await userEvent.type(screen.getByLabelText(/Confirm Password/), 'newpass1')
  await userEvent.click(screen.getByRole('button', { name: 'Update Password' }))
  expect(await screen.findByText('Password updated')).toBeInTheDocument()
  expect(body).toEqual({ password: 'newpass1' })
})

it('flips has_password after a successful password set (query invalidation)', async () => {
  let served = false
  server.use(http.get(`${P}/me`, () => {
    const p = { ...portalFixtures.profile, has_password: served }
    served = true
    return HttpResponse.json(p)
  }))
  server.use(http.post(`${P}/me/password`, () => new HttpResponse(null, { status: 204 })))
  mount()
  expect(await screen.findByText('Set a password')).toBeInTheDocument()
  await userEvent.type(screen.getByLabelText(/New Password/), 'newpass1')
  await userEvent.type(screen.getByLabelText(/Confirm Password/), 'newpass1')
  await userEvent.click(screen.getByRole('button', { name: 'Update Password' }))
  expect(await screen.findByText('Password')).toBeInTheDocument()
  expect(screen.getByLabelText(/Current Password/)).toBeInTheDocument()
})

it('shows a field error on mismatched new and confirm passwords without calling the API', async () => {
  let calls = 0
  server.use(http.post(`${P}/me/password`, () => { calls++; return new HttpResponse(null, { status: 204 }) }))
  mount()
  await userEvent.type(await screen.findByLabelText(/Current Password/), 'oldpass1')
  await userEvent.type(screen.getByLabelText(/New Password/), 'newpass1')
  await userEvent.type(screen.getByLabelText(/Confirm Password/), 'different')
  await userEvent.click(screen.getByRole('button', { name: 'Update Password' }))
  expect(await screen.findByText(/do not match/)).toBeInTheDocument()
  expect(calls).toBe(0)
})

it('scrolls the password section into view and focuses it when the hash is #password', async () => {
  const scroll = vi.fn()
  Element.prototype.scrollIntoView = scroll
  mount('/portal/profile#password')
  await screen.findByLabelText(/Current Password/)
  expect(scroll).toHaveBeenCalled()
  expect(screen.getByLabelText(/Current Password/)).toHaveFocus()
})
