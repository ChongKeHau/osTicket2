import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StrictMode } from 'react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { portal } from '../../api/portalClient'
import { makeQueryClient } from '../../test/render'
import { server } from '../../test/setup'
import { PortalAuthProvider, usePortalAuth } from '../PortalAuthContext'
import { PortalShell } from '../PortalShell'
import { TokenPage } from './TokenPage'

beforeEach(() => portal.tokens.clear())

function Where({ label }: { label: string }) {
  const { pathname, hash } = useLocation()
  const { status, isGuest } = usePortalAuth()
  return <h2>{`${label} ${pathname}${hash} ${status} guest:${String(isGuest)}`}</h2>
}

function mount(token: string) {
  return render(
    <StrictMode>
      <QueryClientProvider client={makeQueryClient()}>
        <MemoryRouter initialEntries={[`/portal/t/${token}`]}>
          <PortalAuthProvider>
            <Routes>
              <Route path="/portal" element={<PortalShell />}>
                <Route index element={<Where label="landing" />} />
                <Route path="t/:token" element={<TokenPage />} />
                <Route path="login" element={<Where label="login" />} />
                <Route path="open" element={<Where label="open" />} />
                <Route path="tickets" element={<Where label="list" />} />
                <Route path="tickets/:id" element={<Where label="ticket" />} />
                <Route path="profile" element={<Where label="profile" />} />
              </Route>
            </Routes>
          </PortalAuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    </StrictMode>,
  )
}

function countExchanges() {
  const calls: string[] = []
  server.events.on('request:start', ({ request }) => {
    if (new URL(request.url).pathname === '/api/v1/portal/auth/exchange') calls.push(request.method)
  })
  return calls
}

afterEach(() => server.events.removeAllListeners())

it('good-signin: exchanges once under StrictMode, adopts the session, goes to the ticket list', async () => {
  const calls = countExchanges()
  mount('good-signin')
  expect(await screen.findByRole('heading', { name: 'list /portal/tickets authenticated guest:false' })).toBeInTheDocument()
  expect(calls).toHaveLength(1)
})

it('good-access: adopts a guest session and goes to its ticket', async () => {
  mount('good-access')
  expect(await screen.findByRole('heading', { name: 'ticket /portal/tickets/7 authenticated guest:true' })).toBeInTheDocument()
})

it('good-confirm: goes to the profile password section with a flash', async () => {
  mount('good-confirm')
  expect(await screen.findByRole('heading', { name: 'profile /portal/profile#password authenticated guest:false' })).toBeInTheDocument()
  expect(screen.getByText('Email confirmed — set your password')).toBeInTheDocument()
})

it('good-reset: goes to the profile password section', async () => {
  mount('good-reset')
  expect(await screen.findByRole('heading', { name: 'profile /portal/profile#password authenticated guest:false' })).toBeInTheDocument()
})

it('a 410 shows the expired message with recovery buttons', async () => {
  mount('stale-token')
  expect(await screen.findByText('This link has expired or was already used')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Open a new ticket' })).toHaveAttribute('href', '/portal/open')
  await userEvent.click(screen.getByRole('link', { name: 'Email me a new sign-in link' }))
  expect(await screen.findByRole('heading', { name: /^login \/portal\/login anonymous/ })).toBeInTheDocument()
})
