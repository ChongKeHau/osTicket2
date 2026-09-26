import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import type { ReactElement, ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { REFRESH_KEY } from '../api/client'
import { AuthProvider } from '../auth/AuthContext'
import { RequireAuth } from '../auth/RequireAuth'

/** Renders `ui` inside an AuthProvider once the session (from the `/me` handler) has been restored. */
export function renderAuthed(ui: ReactElement, opts: { route?: string } = {}) {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  return renderWithProviders(<AuthProvider><Routes><Route element={<RequireAuth />}><Route path="*" element={ui} /></Route></Routes></AuthProvider>, opts)
}

export function makeQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } } })
}

export function renderWithProviders(ui: ReactElement, { route = '/' }: { route?: string } = {}) {
  const client = makeQueryClient()
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[route]}>{children}</MemoryRouter>
      </QueryClientProvider>
    )
  }
  return { client, ...render(ui, { wrapper: Wrapper }) }
}
