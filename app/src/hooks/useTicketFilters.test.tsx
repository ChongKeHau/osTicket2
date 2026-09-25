import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { useTicketFilters } from './useTicketFilters'

function wrapper(route: string) {
  return ({ children }: { children: ReactNode }) => <MemoryRouter initialEntries={[route]}>{children}</MemoryRouter>
}

test('defaults when the URL is empty', () => {
  const { result } = renderHook(() => useTicketFilters(), { wrapper: wrapper('/tickets') })
  expect(result.current.filter).toEqual({ page: 1, page_size: 25, sort: '-last_message_at' })
})

test('reads valid params and ignores junk', () => {
  const { result } = renderHook(() => useTicketFilters(), { wrapper: wrapper('/tickets?state=open&status=3&assigned_to=me&q=printer&sort=priority&page=2&page_size=50') })
  expect(result.current.filter).toEqual({ state: 'open', status: 3, assigned_to: 'me', q: 'printer', sort: 'priority', page: 2, page_size: 50 })
  const junk = renderHook(() => useTicketFilters(), { wrapper: wrapper('/tickets?state=weird&status=abc&page=abc&page_size=999&sort=hack&dept_id=-1') })
  expect(junk.result.current.filter).toEqual({ page: 1, page_size: 25, sort: '-last_message_at' })
})

test('set() updates params and resets page unless page itself changes', () => {
  const { result } = renderHook(() => useTicketFilters(), { wrapper: wrapper('/tickets?page=3') })
  act(() => result.current.set({ state: 'closed' }))
  expect(result.current.filter.page).toBe(1)
  expect(result.current.filter.state).toBe('closed')
  act(() => result.current.set({ page: 4 }))
  expect(result.current.filter.page).toBe(4)
  act(() => result.current.set({ state: undefined }))
  expect(result.current.filter.state).toBeUndefined()
})
