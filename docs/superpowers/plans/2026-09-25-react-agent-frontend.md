# React Agent Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the React single-page app in `app/` that lets an agent log in, work the ticket queue, read and reply to threads with attachments, change status, assign, transfer, and create tickets against the Go API at `/api/v1`.

**Architecture:** Vite + React + TypeScript SPA. `src/api` is the only module that calls `fetch`; it holds the typed request wrapper with refresh-on-401, and one file per API area. `AuthProvider` owns session state; `RequireAuth` guards routes. Pages compose small components and use TanStack Query for reads and mutations with cache invalidation. Tests run in Vitest with Testing Library and MSW mocking every endpoint.

**Tech Stack:** Node 24, npm 11, Vite (current major, 5+), React 18+, TypeScript strict, react-router-dom 6, @tanstack/react-query 5, dompurify, Vitest, @testing-library/react, @testing-library/user-event, @testing-library/jest-dom, msw 2, jsdom, ESLint.

**Spec:** `docs/superpowers/specs/2026-09-25-react-agent-frontend-design.md`

## Global Constraints

- The app lives in `app/` at the repository root. All `npm` commands run from `app/`.
- `src/api` is the only place that calls `fetch`. No `any` in `src/api`. Types mirror the API JSON field names exactly (`snake_case`).
- Access token in memory only; refresh token in `localStorage` under the key `ticket.refresh_token`. On 401 (except `/auth/*` paths) the client refreshes once and retries once; concurrent 401s share one refresh promise; a failed refresh clears the session and routes to `/login` with a "session expired" notice.
- API base path `/api/v1`; dev server on port 5173 proxies `/api` to `http://localhost:8080`.
- Error envelope `{error: {code, message, fields}}` becomes `ApiError {status, code, message, fields}`; unparsable responses become `ApiError` with code `network`.
- Routes: `/login`, `/tickets`, `/tickets/new`, `/tickets/:id`, `/` redirects to `/tickets`, `*` NotFound. All but `/login` behind `RequireAuth`.
- List filters live in URL search params: `state`, `status`, `dept_id`, `assigned_to`, `q` (debounced 300 ms), `sort`, `page`, `page_size`.
- Mutations invalidate `['ticket', id]`, `['thread', id]`, `['events', id]`, `['tickets']`.
- HTML bodies are rendered only after `DOMPurify.sanitize`; `text` bodies render in a preformatted block.
- Replies and notes post `format: "text"`.
- `npm test`, `npm run lint`, `npm run build` must pass at the end of every task.
- Every commit message ends with the trailer line `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Work on the existing feature branch `go-ticket-api` in the current worktree (already isolated).

## Review Focus

1. **Two requests hit 401 at the same time after the access token expires.** Exactly one `/auth/refresh` call must happen and both requests must succeed on retry. Test in Task 2.
2. **Refresh token in storage is stale or revoked on reload.** The app must show the login page with the "session expired" notice, never a blank screen or a loop. Test in Task 4.
3. **A thread entry body contains a `<script>` or `onerror` attribute.** It must render inert. Test in Task 7.
4. **Filter query params contain junk** (`page=abc`, `state=weird`). The page must fall back to defaults instead of crashing or sending junk to the API. Test in Task 5.
5. **User submits a reply while an upload is still in progress.** The submit button must be disabled until pending uploads finish, so `file_ids` is never incomplete. Test in Task 8.

---

## File structure

```
app/
  package.json, vite.config.ts, tsconfig.json, tsconfig.node.json, index.html, eslint config, .gitignore
  src/main.tsx, src/App.tsx, src/vite-env.d.ts
  src/api/client.ts, types.ts, auth.ts, tickets.ts, reference.ts, files.ts
  src/auth/AuthContext.tsx, RequireAuth.tsx
  src/pages/LoginPage.tsx, TicketListPage.tsx, TicketDetailPage.tsx, NewTicketPage.tsx, NotFoundPage.tsx
  src/components/Layout.tsx, LoadingScreen.tsx, ErrorBanner.tsx, StatusBadge.tsx, Pagination.tsx,
    FilterBar.tsx, TicketRow.tsx, TicketHeader.tsx, ThreadEntry.tsx, AttachmentList.tsx,
    FileUpload.tsx, Composer.tsx, EventsPanel.tsx
  src/hooks/useReferenceData.ts, useTicketFilters.ts, useDebouncedValue.ts
  src/lib/format.ts            date formatting
  src/styles/global.css, *.module.css
  src/test/setup.ts, handlers.ts, fixtures.ts, render.tsx
```

---

### Task 1: Scaffold, tooling, test harness

**Files:**
- Create: `app/` via `npm create vite`, then `app/vite.config.ts`, `app/tsconfig.json`, `app/src/styles/global.css`, `app/src/api/types.ts` (full contents from Task 2 Step 1), `app/src/test/setup.ts`, `app/src/test/render.tsx`, `app/src/test/fixtures.ts`, `app/src/test/handlers.ts`, `app/src/App.test.tsx`
- Modify: `app/package.json` scripts, `app/src/main.tsx`, `app/src/App.tsx`

**Interfaces:**
- Consumes: nothing.
- Produces: `renderWithProviders(ui, {route?})` from `src/test/render.tsx` (wraps in `QueryClientProvider` and `MemoryRouter`, returns `{client, ...renderResult}`), `makeQueryClient()`, MSW `server` from `src/test/setup.ts`, fixtures `staffProfileFixture`, `sessionFixture`, `ticketFixture`, `entryFixtures`, `eventFixtures`, `referenceFixtures` from `src/test/fixtures.ts`, `handlers` from `src/test/handlers.ts`.

- [ ] **Step 1: Create the project**

From the repository root:

```bash
npm create vite@latest app -- --template react-ts
cd app
npm install
npm install react-router-dom @tanstack/react-query dompurify
npm install -D vitest jsdom @testing-library/react @testing-library/user-event @testing-library/jest-dom msw @types/dompurify
```

If `@types/dompurify` reports that dompurify ships its own types, skip it.

- [ ] **Step 2: Configure Vite, TypeScript, scripts**

`app/vite.config.ts`:

```ts
/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: false } },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: { modules: { classNameStrategy: 'non-scoped' } },
  },
})
```

In the app tsconfig that covers `src` (the template names it `tsconfig.app.json` or `tsconfig.json`), ensure `"strict": true`, add `"noUncheckedIndexedAccess": true`, and add `"types": ["vitest/globals", "@testing-library/jest-dom"]` under `compilerOptions`.

In `app/package.json` set scripts:

```json
"scripts": {
  "dev": "vite",
  "build": "tsc -b && vite build",
  "preview": "vite preview",
  "test": "vitest run",
  "test:watch": "vitest",
  "lint": "eslint ."
}
```

Ensure `app/.gitignore` lists `node_modules`, `dist`, `coverage`.

- [ ] **Step 3: Global stylesheet**

`app/src/styles/global.css`:

```css
:root {
  --bg: #f6f7f9; --panel: #ffffff; --text: #1f2933; --muted: #6b7280; --border: #d9dde3;
  --accent: #2563eb; --accent-text: #ffffff; --danger: #b91c1c; --ok: #15803d; --warn: #b45309;
  --radius: 6px; font-family: system-ui, -apple-system, "Segoe UI", sans-serif; font-size: 14px;
}
* { box-sizing: border-box; }
body { margin: 0; background: var(--bg); color: var(--text); }
a { color: var(--accent); text-decoration: none; }
button, select, input, textarea { font: inherit; }
button { cursor: pointer; border: 1px solid var(--border); background: var(--panel); border-radius: var(--radius); padding: 6px 12px; }
button.primary { background: var(--accent); color: var(--accent-text); border-color: var(--accent); }
button:disabled { opacity: 0.6; cursor: not-allowed; }
input, select, textarea { border: 1px solid var(--border); border-radius: var(--radius); padding: 6px 8px; background: var(--panel); }
table { border-collapse: collapse; width: 100%; background: var(--panel); }
th, td { text-align: left; padding: 8px 10px; border-bottom: 1px solid var(--border); }
th button { border: none; background: none; padding: 0; font-weight: 600; }
.field { display: flex; flex-direction: column; gap: 4px; margin-bottom: 12px; }
.field-error { color: var(--danger); font-size: 12px; }
.panel { background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius); padding: 16px; margin-bottom: 16px; }
.muted { color: var(--muted); }
.row { display: flex; gap: 12px; align-items: center; flex-wrap: wrap; }
.pre { white-space: pre-wrap; font-family: inherit; margin: 0; }
```

Import it once in `app/src/main.tsx`.

- [ ] **Step 4: Test harness**

`app/src/test/fixtures.ts`:

```ts
import type { Department, Entry, Event, Priority, Session, Staff, StaffProfile, Status, Ticket, Topic } from '../api/types'

export const staffProfileFixture: StaffProfile = {
  id: 1, username: 'agent', email: 'agent@example.test', first_name: 'Ann', last_name: 'Agent',
  is_admin: false, department_ids: [1],
}

export const sessionFixture: Session = {
  access_token: 'access-1', refresh_token: 'refresh-1', expires_in: 900, staff: staffProfileFixture,
}

export const ticketFixture: Ticket = {
  id: 7, number: '000007', subject: 'Printer on fire', status: { id: 1, name: 'Open' }, state: 'open',
  department: { id: 1, name: 'Support' }, topic: { id: 1, name: 'General Inquiry' },
  priority: { id: 2, name: 'normal' }, assignee: null, requester_name: 'Pat', requester_email: 'pat@example.test',
  source: 'phone', is_answered: false, due_at: null, closed_at: null,
  last_message_at: '2026-09-25T10:00:00Z', last_response_at: null, extra: {},
  created_at: '2026-09-25T10:00:00Z', updated_at: '2026-09-25T10:00:00Z',
}

export const entryFixtures: Entry[] = [
  { id: 1, ticket_id: 7, type: 'message', staff_id: null, poster: 'Pat', title: null, body: '<p>Help</p>',
    format: 'html', parent_id: null, attachments: [], created_at: '2026-09-25T10:00:00Z' },
  { id: 2, ticket_id: 7, type: 'response', staff_id: 1, poster: 'Ann Agent', title: null, body: 'On it',
    format: 'text', parent_id: null, attachments: [{ file_id: 3, name: 'log.txt', mime: 'text/plain', size: 12 }],
    created_at: '2026-09-25T10:05:00Z' },
]

export const eventFixtures: Event[] = [
  { id: 1, ticket_id: 7, staff: { id: 1, name: 'Ann Agent' }, kind: 'created', data: { number: '000007' }, created_at: '2026-09-25T10:00:00Z' },
]

export const referenceFixtures = {
  priorities: [{ id: 1, name: 'low', urgency: 1, color: '' }, { id: 2, name: 'normal', urgency: 2, color: '' }, { id: 3, name: 'high', urgency: 3, color: '' }] as Priority[],
  statuses: [{ id: 1, name: 'Open', state: 'open', sort_order: 1 }, { id: 2, name: 'Resolved', state: 'resolved', sort_order: 2 }, { id: 3, name: 'Closed', state: 'closed', sort_order: 3 }] as Status[],
  departments: [{ id: 1, name: 'Support', is_public: true, manager_id: null }, { id: 2, name: 'Billing', is_public: true, manager_id: null }] as Department[],
  topics: [{ id: 1, name: 'General Inquiry', dept_id: 1, priority_id: 2, is_active: true, sort_order: 1 }, { id: 2, name: 'Refunds', dept_id: 2, priority_id: 3, is_active: true, sort_order: 2 }] as Topic[],
  staff: [
    { id: 1, username: 'agent', email: 'agent@example.test', first_name: 'Ann', last_name: 'Agent', is_admin: false, is_active: true, primary_dept_id: 1, department_ids: [1] },
    { id: 2, username: 'bob', email: 'bob@example.test', first_name: 'Bob', last_name: 'Billing', is_admin: false, is_active: true, primary_dept_id: 2, department_ids: [2] },
    { id: 3, username: 'root', email: 'root@example.test', first_name: 'Root', last_name: 'Admin', is_admin: true, is_active: true, primary_dept_id: 1, department_ids: [1] },
  ] as Staff[],
}
```

`app/src/test/handlers.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { entryFixtures, eventFixtures, referenceFixtures, sessionFixture, staffProfileFixture, ticketFixture } from './fixtures'

const unauthorized = () => HttpResponse.json({ error: { code: 'unauthorized', message: 'authentication required' } }, { status: 401 })

export const handlers = [
  http.post('/api/v1/auth/login', async ({ request }) => {
    const body = (await request.json()) as { username: string; password: string }
    if (body.username === 'agent' && body.password === 'password1') return HttpResponse.json(sessionFixture)
    return unauthorized()
  }),
  http.post('/api/v1/auth/refresh', async ({ request }) => {
    const body = (await request.json()) as { refresh_token: string }
    if (body.refresh_token.startsWith('refresh')) {
      return HttpResponse.json({ ...sessionFixture, access_token: 'access-2', refresh_token: 'refresh-2' })
    }
    return unauthorized()
  }),
  http.post('/api/v1/auth/logout', () => new HttpResponse(null, { status: 204 })),
  http.get('/api/v1/me', () => HttpResponse.json(staffProfileFixture)),
  http.get('/api/v1/priorities', () => HttpResponse.json({ items: referenceFixtures.priorities })),
  http.get('/api/v1/statuses', () => HttpResponse.json({ items: referenceFixtures.statuses })),
  http.get('/api/v1/departments', () => HttpResponse.json({ items: referenceFixtures.departments })),
  http.get('/api/v1/topics', () => HttpResponse.json({ items: referenceFixtures.topics })),
  http.get('/api/v1/staff', () => HttpResponse.json({ items: referenceFixtures.staff })),
  http.get('/api/v1/tickets', () => HttpResponse.json({ items: [ticketFixture], page: 1, page_size: 25, total: 1 })),
  http.get('/api/v1/tickets/:id', ({ params }) =>
    params.id === '7' ? HttpResponse.json(ticketFixture)
      : HttpResponse.json({ error: { code: 'not_found', message: 'ticket not found' } }, { status: 404 })),
  http.get('/api/v1/tickets/:id/thread', () => HttpResponse.json({ items: entryFixtures, next_after: null })),
  http.get('/api/v1/tickets/:id/events', () => HttpResponse.json({ items: eventFixtures })),
  http.post('/api/v1/tickets', () => HttpResponse.json({ ...ticketFixture, id: 8, number: '000008' }, { status: 201 })),
  http.patch('/api/v1/tickets/:id', () => HttpResponse.json(ticketFixture)),
  http.post('/api/v1/tickets/:id/reply', () => HttpResponse.json({ ...entryFixtures[1], id: 9 }, { status: 201 })),
  http.post('/api/v1/tickets/:id/notes', () => HttpResponse.json({ ...entryFixtures[1], id: 10, type: 'note' }, { status: 201 })),
  http.post('/api/v1/tickets/:id/status', () => HttpResponse.json({ ...ticketFixture, status: { id: 3, name: 'Closed' }, state: 'closed' })),
  http.post('/api/v1/tickets/:id/assign', () => HttpResponse.json({ ...ticketFixture, assignee: { id: 1, name: 'Ann Agent' } })),
  http.post('/api/v1/tickets/:id/transfer', () => HttpResponse.json({ ...ticketFixture, department: { id: 2, name: 'Billing' } })),
  http.post('/api/v1/files', () => HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 })),
  http.get('/api/v1/files/:id', () => new HttpResponse('hello', { headers: { 'Content-Type': 'text/plain' } })),
]
```

`app/src/test/setup.ts`:

```ts
import '@testing-library/jest-dom/vitest'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll } from 'vitest'
import { handlers } from './handlers'

export const server = setupServer(...handlers)

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => {
  server.resetHandlers()
  localStorage.clear()
})
afterAll(() => server.close())
```

`app/src/test/render.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import type { ReactElement, ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'

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
```

Create `app/src/api/types.ts` with the full contents shown in Task 2 Step 1 (a pure type file with no dependencies) so the fixtures compile.

- [ ] **Step 5: Smoke test**

`app/src/App.test.tsx`:

```tsx
import { screen } from '@testing-library/react'
import { renderWithProviders } from './test/render'
import App from './App'

test('renders the app title', () => {
  renderWithProviders(<App />)
  expect(screen.getByText(/ticket desk/i)).toBeInTheDocument()
})
```

Replace `app/src/App.tsx` with:

```tsx
export default function App() {
  return <h1>Ticket Desk</h1>
}
```

and `app/src/main.tsx` with:

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './styles/global.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
```

Delete the template's `App.css`, `index.css`, and `src/assets/` if present.

- [ ] **Step 6: Run everything**

Run: `npm test && npm run lint && npm run build`
Expected: 1 test passes, lint clean, build succeeds.

- [ ] **Step 7: Commit**

```bash
git add app
git commit -m "feat(app): scaffold React frontend with Vite, Vitest and MSW harness" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: API types and client with refresh-on-401

**Files:**
- Create or replace: `app/src/api/types.ts`, `app/src/api/client.ts`, `app/src/api/client.test.ts`

**Interfaces:**
- Produces: all types below; `ApiError`; `tokens` (`access` getter, `getRefresh()`, `setSession(s)`, `clear()`, `setOnSessionLost(fn)`); `fetchWithAuth(method, path, init?) => Promise<Response>`; `request<T>(method, path, opts?) => Promise<T>`; `refreshSession() => Promise<boolean>`; `REFRESH_KEY`.

- [ ] **Step 1: Types**

`app/src/api/types.ts`:

```ts
export interface Ref { id: number; name: string }
export type TicketState = 'open' | 'resolved' | 'closed'

export interface Ticket {
  id: number; number: string; subject: string; status: Ref; state: TicketState
  department: Ref; topic: Ref | null; priority: Ref; assignee: Ref | null
  requester_name: string; requester_email: string; source: string; is_answered: boolean
  due_at: string | null; closed_at: string | null; last_message_at: string; last_response_at: string | null
  extra: Record<string, unknown>; created_at: string; updated_at: string
}

export interface ListResponse<T> { items: T[]; page: number; page_size: number; total: number }

export interface AttachmentRef { file_id: number; name: string; mime: string; size: number }
export type EntryType = 'message' | 'response' | 'note'
export interface Entry {
  id: number; ticket_id: number; type: EntryType; staff_id: number | null; poster: string
  title: string | null; body: string; format: 'html' | 'text'; parent_id: number | null
  attachments: AttachmentRef[]; created_at: string
}
export interface Thread { items: Entry[]; next_after: number | null }
export interface Event { id: number; ticket_id: number; staff: Ref | null; kind: string; data: Record<string, unknown>; created_at: string }

export interface StaffProfile { id: number; username: string; email: string; first_name: string; last_name: string; is_admin: boolean; department_ids: number[] }
export interface Session { access_token: string; refresh_token: string; expires_in: number; staff: StaffProfile }

export interface Priority { id: number; name: string; urgency: number; color: string }
export interface Status { id: number; name: string; state: TicketState; sort_order: number }
export interface Department { id: number; name: string; is_public: boolean; manager_id: number | null }
export interface Topic { id: number; name: string; dept_id: number | null; priority_id: number | null; is_active: boolean; sort_order: number }
export interface Staff { id: number; username: string; email: string; first_name: string; last_name: string; is_admin: boolean; is_active: boolean; primary_dept_id: number; department_ids: number[] }
export interface FileInfo { id: number; name: string; mime: string; size: number }

export interface ListFilter {
  state?: TicketState; status?: number; dept_id?: number; assigned_to?: string; q?: string
  sort?: string; page: number; page_size: number
}
export interface CreateTicketInput {
  subject: string; message: string; message_format?: 'html' | 'text'; requester_name?: string; requester_email: string
  dept_id?: number; topic_id?: number; priority_id?: number; source?: string; due_at?: string | null; file_ids?: number[]
}
export interface UpdateTicketInput {
  subject?: string; priority_id?: number; topic_id?: number | null; due_at?: string | null
  requester_name?: string; requester_email?: string
}
export interface ReplyInput { body: string; format: 'html' | 'text'; status_id?: number; file_ids?: number[] }
export interface NoteInput { title?: string; body: string; format: 'html' | 'text'; file_ids?: number[] }

export interface ApiErrorBody { error: { code: string; message: string; fields?: Record<string, string> } }
```

- [ ] **Step 2: Failing client tests**

`app/src/api/client.test.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { sessionFixture } from '../test/fixtures'
import { ApiError, REFRESH_KEY, refreshSession, request, tokens } from './client'

beforeEach(() => tokens.clear())

test('attaches bearer header and parses JSON', async () => {
  let auth = ''
  server.use(http.get('/api/v1/ping', ({ request: req }) => { auth = req.headers.get('authorization') ?? ''; return HttpResponse.json({ ok: true }) }))
  tokens.setSession(sessionFixture)
  const out = await request<{ ok: boolean }>('GET', '/ping')
  expect(out.ok).toBe(true)
  expect(auth).toBe('Bearer access-1')
})

test('maps the error envelope to ApiError', async () => {
  server.use(http.post('/api/v1/tickets', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { subject: 'required' } } }, { status: 400 })))
  await expect(request('POST', '/tickets', { body: {} })).rejects.toMatchObject({ status: 400, code: 'validation_failed', fields: { subject: 'required' } })
  const err = await request('POST', '/tickets', { body: {} }).catch((e: unknown) => e)
  expect(err).toBeInstanceOf(ApiError)
})

test('non-JSON failure becomes a network ApiError', async () => {
  server.use(http.get('/api/v1/boom', () => new HttpResponse('<html>bad gateway</html>', { status: 502 })))
  await expect(request('GET', '/boom')).rejects.toMatchObject({ status: 502, code: 'network' })
})

test('appends query params and drops empty ones', async () => {
  let url = ''
  server.use(http.get('/api/v1/tickets', ({ request: req }) => { url = req.url; return HttpResponse.json({ items: [] }) }))
  await request('GET', '/tickets', { query: { page: 2, q: '', state: undefined, sort: '-priority' } })
  expect(new URL(url).search).toBe('?page=2&sort=-priority')
})

test('refreshes once on 401 and retries; concurrent 401s share one refresh', async () => {
  let refreshCalls = 0
  let pingCalls = 0
  server.use(
    http.post('/api/v1/auth/refresh', () => { refreshCalls++; return HttpResponse.json({ ...sessionFixture, access_token: 'access-2', refresh_token: 'refresh-2' }) }),
    http.get('/api/v1/ping', ({ request: req }) => {
      pingCalls++
      if (req.headers.get('authorization') !== 'Bearer access-2') {
        return HttpResponse.json({ error: { code: 'unauthorized', message: 'authentication required' } }, { status: 401 })
      }
      return HttpResponse.json({ ok: true })
    }),
  )
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  const [a, b] = await Promise.all([request<{ ok: boolean }>('GET', '/ping'), request<{ ok: boolean }>('GET', '/ping')])
  expect(a.ok && b.ok).toBe(true)
  expect(refreshCalls).toBe(1)
  expect(pingCalls).toBe(4)
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-2')
  expect(tokens.access).toBe('access-2')
})

test('failed refresh clears the session and notifies', async () => {
  const lost = vi.fn()
  tokens.setOnSessionLost(lost)
  server.use(
    http.post('/api/v1/auth/refresh', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })),
    http.get('/api/v1/ping', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })),
  )
  localStorage.setItem(REFRESH_KEY, 'refresh-stale')
  await expect(request('GET', '/ping')).rejects.toMatchObject({ status: 401 })
  expect(lost).toHaveBeenCalledTimes(1)
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
  expect(await refreshSession()).toBe(false)
  tokens.setOnSessionLost(null)
})

test('does not try to refresh for /auth/* paths', async () => {
  let refreshCalls = 0
  server.use(http.post('/api/v1/auth/refresh', () => { refreshCalls++; return HttpResponse.json(sessionFixture) }))
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  await expect(request('POST', '/auth/login', { body: { username: 'x', password: 'y' } })).rejects.toMatchObject({ status: 401 })
  expect(refreshCalls).toBe(0)
})

test('returns undefined for 204', async () => {
  server.use(http.post('/api/v1/auth/logout', () => new HttpResponse(null, { status: 204 })))
  await expect(request('POST', '/auth/logout', { body: { refresh_token: 'r' } })).resolves.toBeUndefined()
})
```

- [ ] **Step 3: Run to verify it fails**

Run: `npm test -- src/api/client.test.ts`
Expected: FAIL, cannot resolve `./client`.

- [ ] **Step 4: Implement the client**

`app/src/api/client.ts`:

```ts
import type { ApiErrorBody, Session } from './types'

export const REFRESH_KEY = 'ticket.refresh_token'
const BASE = '/api/v1'

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly fields: Record<string, string> = {},
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

let accessToken: string | null = null
let refreshing: Promise<boolean> | null = null
let sessionLostHandler: (() => void) | null = null

function safeStorage<T>(fn: () => T, fallback: T): T {
  try { return fn() } catch { return fallback }
}

export const tokens = {
  get access(): string | null { return accessToken },
  getRefresh(): string | null { return safeStorage(() => localStorage.getItem(REFRESH_KEY), null) },
  setSession(s: Session): void {
    accessToken = s.access_token
    safeStorage(() => localStorage.setItem(REFRESH_KEY, s.refresh_token), undefined)
  },
  clear(): void {
    accessToken = null
    safeStorage(() => localStorage.removeItem(REFRESH_KEY), undefined)
  },
  setOnSessionLost(fn: (() => void) | null): void { sessionLostHandler = fn },
}

export type Query = Record<string, string | number | boolean | undefined | null>

function buildUrl(path: string, query?: Query): string {
  const url = new URL(BASE + path, window.location.origin)
  if (query) {
    for (const [k, v] of Object.entries(query)) {
      if (v === undefined || v === null || v === '') continue
      url.searchParams.set(k, String(v))
    }
  }
  return url.toString()
}

export async function refreshSession(): Promise<boolean> {
  if (refreshing) return refreshing
  const raw = tokens.getRefresh()
  if (!raw) return false
  refreshing = (async () => {
    try {
      const res = await fetch(buildUrl('/auth/refresh'), {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ refresh_token: raw }),
      })
      if (!res.ok) throw new Error('refresh failed')
      tokens.setSession((await res.json()) as Session)
      return true
    } catch {
      tokens.clear()
      sessionLostHandler?.()
      return false
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

export interface FetchInit { body?: unknown; formData?: FormData; query?: Query; headers?: Record<string, string> }

export async function fetchWithAuth(method: string, path: string, init: FetchInit = {}, retry = true): Promise<Response> {
  const headers: Record<string, string> = { Accept: 'application/json', ...init.headers }
  if (accessToken) headers['Authorization'] = `Bearer ${accessToken}`
  let body: BodyInit | undefined
  if (init.formData) body = init.formData
  else if (init.body !== undefined) { headers['Content-Type'] = 'application/json'; body = JSON.stringify(init.body) }
  const res = await fetch(buildUrl(path, init.query), { method, headers, body })
  if (res.status === 401 && retry && !path.startsWith('/auth/')) {
    const ok = await refreshSession()
    if (ok) return fetchWithAuth(method, path, init, false)
  }
  return res
}

async function toError(res: Response): Promise<ApiError> {
  try {
    const data = (await res.json()) as Partial<ApiErrorBody>
    if (data.error && typeof data.error.code === 'string') {
      return new ApiError(res.status, data.error.code, data.error.message ?? res.statusText, data.error.fields ?? {})
    }
  } catch { /* not JSON */ }
  return new ApiError(res.status, 'network', `request failed with status ${res.status}`)
}

export async function request<T>(method: string, path: string, opts: FetchInit = {}): Promise<T> {
  let res: Response
  try {
    res = await fetchWithAuth(method, path, opts)
  } catch (e) {
    throw new ApiError(0, 'network', e instanceof Error ? e.message : 'network error')
  }
  if (!res.ok) throw await toError(res)
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}
```

- [ ] **Step 5: Run the client tests**

Run: `npm test -- src/api/client.test.ts`
Expected: PASS (8 tests). Then `npm run lint && npm run build` clean.

- [ ] **Step 6: Commit**

```bash
git add app/src/api
git commit -m "feat(app): typed API client with refresh-on-401 and error envelope mapping" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: API endpoint modules

**Files:**
- Create: `app/src/api/auth.ts`, `app/src/api/tickets.ts`, `app/src/api/reference.ts`, `app/src/api/files.ts`, `app/src/api/endpoints.test.ts`

**Interfaces:**
- Consumes: `request`, `fetchWithAuth`, `tokens`, `ApiError` from `client.ts`; types.
- Produces: `login(username, password): Promise<Session>` (stores tokens), `logout(): Promise<void>`, `me(): Promise<StaffProfile>`; `listTickets(f: ListFilter)`, `getTicket(id)`, `createTicket(input)`, `updateTicket(id, input)`, `reply(id, input)`, `note(id, input)`, `getThread(id, after?, limit?)`, `setStatus(id, statusId)`, `assign(id, staffId | null)`, `transfer(id, deptId)`, `getEvents(id): Promise<Event[]>`; `listPriorities()`, `listStatuses()`, `listDepartments()`, `listTopics()`, `listStaff()` each resolving to the plain array; `uploadFile(file: File): Promise<FileInfo>`, `downloadFile(id: number, name: string): Promise<void>`.

- [ ] **Step 1: Failing tests**

`app/src/api/endpoints.test.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { sessionFixture } from '../test/fixtures'
import { login, logout, me } from './auth'
import { REFRESH_KEY, tokens } from './client'
import { downloadFile, uploadFile } from './files'
import { listStaff, listStatuses } from './reference'
import { assign, listTickets, reply } from './tickets'

beforeEach(() => tokens.clear())

test('login stores tokens; logout posts refresh token and clears', async () => {
  const s = await login('agent', 'password1')
  expect(s.staff.username).toBe('agent')
  expect(tokens.access).toBe('access-1')
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-1')
  let sent = ''
  server.use(http.post('/api/v1/auth/logout', async ({ request }) => { sent = ((await request.json()) as { refresh_token: string }).refresh_token; return new HttpResponse(null, { status: 204 }) }))
  await logout()
  expect(sent).toBe('refresh-1')
  expect(tokens.access).toBeNull()
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

test('logout clears even when the API call fails', async () => {
  tokens.setSession(sessionFixture)
  server.use(http.post('/api/v1/auth/logout', () => HttpResponse.json({ error: { code: 'internal', message: 'x' } }, { status: 500 })))
  await logout()
  expect(tokens.access).toBeNull()
})

test('me returns the profile', async () => {
  tokens.setSession(sessionFixture)
  expect((await me()).department_ids).toEqual([1])
})

test('listTickets sends filters as query params', async () => {
  let url = ''
  server.use(http.get('/api/v1/tickets', ({ request }) => { url = request.url; return HttpResponse.json({ items: [], page: 2, page_size: 10, total: 0 }) }))
  tokens.setSession(sessionFixture)
  await listTickets({ state: 'open', assigned_to: 'me', q: 'printer', sort: '-priority', page: 2, page_size: 10 })
  const p = new URL(url).searchParams
  expect(p.get('state')).toBe('open'); expect(p.get('assigned_to')).toBe('me'); expect(p.get('q')).toBe('printer')
  expect(p.get('sort')).toBe('-priority'); expect(p.get('page')).toBe('2'); expect(p.get('page_size')).toBe('10')
})

test('reply and assign post the right bodies', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('/api/v1/tickets/7/reply', async ({ request }) => { bodies.push(await request.json()); return HttpResponse.json({ id: 1 }, { status: 201 }) }),
    http.post('/api/v1/tickets/7/assign', async ({ request }) => { bodies.push(await request.json()); return HttpResponse.json({ id: 7 }) }),
  )
  tokens.setSession(sessionFixture)
  await reply(7, { body: 'hi', format: 'text', status_id: 3, file_ids: [42] })
  await assign(7, null)
  expect(bodies[0]).toEqual({ body: 'hi', format: 'text', status_id: 3, file_ids: [42] })
  expect(bodies[1]).toEqual({ staff_id: null })
})

test('reference lists unwrap items', async () => {
  tokens.setSession(sessionFixture)
  expect((await listStatuses()).map((s) => s.state)).toEqual(['open', 'resolved', 'closed'])
  expect((await listStaff()).length).toBe(3)
})

test('uploadFile posts multipart and downloadFile fetches with auth', async () => {
  let contentType = ''
  let auth = ''
  server.use(
    http.post('/api/v1/files', ({ request }) => { contentType = request.headers.get('content-type') ?? ''; return HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 }) }),
    http.get('/api/v1/files/42', ({ request }) => { auth = request.headers.get('authorization') ?? ''; return new HttpResponse('abc', { headers: { 'Content-Type': 'text/plain' } }) }),
  )
  tokens.setSession(sessionFixture)
  const info = await uploadFile(new File(['abc'], 'a.txt', { type: 'text/plain' }))
  expect(info.id).toBe(42)
  expect(contentType).toMatch(/^multipart\/form-data/)
  const createObjectURL = vi.fn(() => 'blob:x')
  const revokeObjectURL = vi.fn()
  Object.assign(URL, { createObjectURL, revokeObjectURL })
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  await downloadFile(42, 'a.txt')
  expect(auth).toBe('Bearer access-1')
  expect(createObjectURL).toHaveBeenCalledTimes(1)
  expect(click).toHaveBeenCalledTimes(1)
  expect(revokeObjectURL).toHaveBeenCalledWith('blob:x')
  click.mockRestore()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npm test -- src/api/endpoints.test.ts`
Expected: FAIL, cannot resolve `./auth`.

- [ ] **Step 3: Implement the modules**

`app/src/api/auth.ts`:

```ts
import { request, tokens } from './client'
import type { Session, StaffProfile } from './types'

export async function login(username: string, password: string): Promise<Session> {
  const s = await request<Session>('POST', '/auth/login', { body: { username, password } })
  tokens.setSession(s)
  return s
}

export async function logout(): Promise<void> {
  const raw = tokens.getRefresh()
  if (raw) {
    try { await request<void>('POST', '/auth/logout', { body: { refresh_token: raw } }) } catch { /* clear locally regardless */ }
  }
  tokens.clear()
}

export function me(): Promise<StaffProfile> {
  return request<StaffProfile>('GET', '/me')
}
```

`app/src/api/tickets.ts`:

```ts
import { request } from './client'
import type { CreateTicketInput, Entry, Event, ListFilter, ListResponse, NoteInput, ReplyInput, Thread, Ticket, UpdateTicketInput } from './types'

export function listTickets(f: ListFilter): Promise<ListResponse<Ticket>> {
  return request('GET', '/tickets', { query: { ...f } })
}
export function getTicket(id: number): Promise<Ticket> { return request('GET', `/tickets/${id}`) }
export function createTicket(input: CreateTicketInput): Promise<Ticket> { return request('POST', '/tickets', { body: input }) }
export function updateTicket(id: number, input: UpdateTicketInput): Promise<Ticket> { return request('PATCH', `/tickets/${id}`, { body: input }) }
export function reply(id: number, input: ReplyInput): Promise<Entry> { return request('POST', `/tickets/${id}/reply`, { body: input }) }
export function note(id: number, input: NoteInput): Promise<Entry> { return request('POST', `/tickets/${id}/notes`, { body: input }) }
export function getThread(id: number, after = 0, limit = 50): Promise<Thread> {
  return request('GET', `/tickets/${id}/thread`, { query: { after: after || undefined, limit } })
}
export function setStatus(id: number, statusId: number): Promise<Ticket> { return request('POST', `/tickets/${id}/status`, { body: { status_id: statusId } }) }
export function assign(id: number, staffId: number | null): Promise<Ticket> { return request('POST', `/tickets/${id}/assign`, { body: { staff_id: staffId } }) }
export function transfer(id: number, deptId: number): Promise<Ticket> { return request('POST', `/tickets/${id}/transfer`, { body: { dept_id: deptId } }) }
export async function getEvents(id: number): Promise<Event[]> {
  return (await request<{ items: Event[] }>('GET', `/tickets/${id}/events`)).items
}
```

`app/src/api/reference.ts`:

```ts
import { request } from './client'
import type { Department, Priority, Staff, Status, Topic } from './types'

async function items<T>(path: string): Promise<T[]> { return (await request<{ items: T[] }>('GET', path)).items }
export const listPriorities = () => items<Priority>('/priorities')
export const listStatuses = () => items<Status>('/statuses')
export const listDepartments = () => items<Department>('/departments')
export const listTopics = () => items<Topic>('/topics')
export const listStaff = () => items<Staff>('/staff')
```

`app/src/api/files.ts`:

```ts
import { ApiError, fetchWithAuth, request } from './client'
import type { FileInfo } from './types'

export function uploadFile(file: File): Promise<FileInfo> {
  const fd = new FormData()
  fd.append('file', file, file.name)
  return request<FileInfo>('POST', '/files', { formData: fd })
}

export async function downloadFile(id: number, name: string): Promise<void> {
  const res = await fetchWithAuth('GET', `/files/${id}`)
  if (!res.ok) throw new ApiError(res.status, res.status === 404 ? 'not_found' : 'network', 'download failed')
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
```

- [ ] **Step 4: Run tests, lint, build**

Run: `npm test && npm run lint && npm run build`
Expected: all pass (16 tests so far).

- [ ] **Step 5: Commit**

```bash
git add app/src/api
git commit -m "feat(app): API modules for auth, tickets, reference data and files" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: AuthProvider, RequireAuth, login page, app shell and routes

**Files:**
- Create: `app/src/auth/AuthContext.tsx`, `app/src/auth/RequireAuth.tsx`, `app/src/components/LoadingScreen.tsx`, `app/src/components/ErrorBanner.tsx`, `app/src/components/Layout.tsx`, `app/src/pages/LoginPage.tsx`, `app/src/pages/NotFoundPage.tsx`, `app/src/lib/format.ts`, `app/src/auth/auth.test.tsx`
- Modify: `app/src/App.tsx`, `app/src/main.tsx`, `app/src/App.test.tsx`

**Interfaces:**
- Consumes: `login`, `logout`, `me` from `api/auth.ts`; `tokens`, `refreshSession` from `api/client.ts`.
- Produces: `AuthProvider`, `useAuth(): { status: 'loading'|'anonymous'|'authenticated'; staff: StaffProfile|null; isAdmin: boolean; departmentIds: number[]; notice: string|null; login(u,p): Promise<void>; logout(): Promise<void> }`; `RequireAuth` (route element rendering `<Outlet/>`); `Layout` (header with app name, staff name, logout; `<Outlet/>`); `LoadingScreen`; `ErrorBanner({error, onRetry?})`; `formatDateTime(iso: string | null): string`; `App` with all routes (placeholder pages for tickets until Tasks 5, 6, 9 replace them).

- [ ] **Step 1: Failing tests**

`app/src/auth/auth.test.tsx`:

```tsx
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../App'
import { REFRESH_KEY, tokens } from '../api/client'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'

beforeEach(() => tokens.clear())

test('anonymous visit to /tickets redirects to /login and back after login', async () => {
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()
  await userEvent.type(screen.getByLabelText(/username/i), 'agent')
  await userEvent.type(screen.getByLabelText(/password/i), 'password1')
  await userEvent.click(screen.getByRole('button', { name: /sign in/i }))
  expect(await screen.findByText(/Ann Agent/)).toBeInTheDocument()
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-1')
  expect(screen.queryByRole('heading', { name: /sign in/i })).not.toBeInTheDocument()
})

test('wrong password shows an error', async () => {
  renderWithProviders(<App />, { route: '/login' })
  await userEvent.type(await screen.findByLabelText(/username/i), 'agent')
  await userEvent.type(screen.getByLabelText(/password/i), 'nope')
  await userEvent.click(screen.getByRole('button', { name: /sign in/i }))
  expect(await screen.findByText(/invalid username or password/i)).toBeInTheDocument()
})

test('reload with a stored refresh token restores the session', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByText(/Ann Agent/)).toBeInTheDocument()
  expect(tokens.access).toBe('access-2')
})

test('stale refresh token on reload lands on login with a notice', async () => {
  localStorage.setItem(REFRESH_KEY, 'stale')
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()
  expect(screen.getByText(/session expired/i)).toBeInTheDocument()
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

test('logout clears storage and returns to login', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  renderWithProviders(<App />, { route: '/tickets' })
  await screen.findByText(/Ann Agent/)
  await userEvent.click(screen.getByRole('button', { name: /sign out/i }))
  expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

test('mid-session refresh failure routes to login with the notice', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  renderWithProviders(<App />, { route: '/tickets' })
  await screen.findByText(/Ann Agent/)
  server.use(
    http.get('/api/v1/tickets', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })),
    http.post('/api/v1/auth/refresh', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })),
  )
  await userEvent.click(screen.getByRole('link', { name: /^tickets$/i }))
  await waitFor(() => expect(screen.getByRole('heading', { name: /sign in/i })).toBeInTheDocument())
  expect(screen.getByText(/session expired/i)).toBeInTheDocument()
})
```

Update `app/src/App.test.tsx` to:

```tsx
import { screen } from '@testing-library/react'
import App from './App'
import { renderWithProviders } from './test/render'

test('unknown route renders not found', async () => {
  localStorage.setItem('ticket.refresh_token', 'refresh-1')
  renderWithProviders(<App />, { route: '/nope' })
  expect(await screen.findByText(/page not found/i)).toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npm test -- src/auth`
Expected: FAIL, cannot resolve `./AuthContext` and friends.

- [ ] **Step 3: Implement**

`app/src/lib/format.ts`:

```ts
const fmt = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : fmt.format(d)
}

export function staffName(s: { first_name: string; last_name: string; username: string }): string {
  const full = `${s.first_name} ${s.last_name}`.trim()
  return full || s.username
}
```

`app/src/components/LoadingScreen.tsx`:

```tsx
export function LoadingScreen({ label = 'Loading…' }: { label?: string }) {
  return <div role="status" aria-live="polite" className="muted" style={{ padding: 24 }}>{label}</div>
}
```

`app/src/components/ErrorBanner.tsx`:

```tsx
import { ApiError } from '../api/client'

export function ErrorBanner({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const message = error instanceof ApiError ? error.message : error instanceof Error ? error.message : 'Something went wrong'
  return (
    <div role="alert" className="panel" style={{ borderColor: 'var(--danger)', color: 'var(--danger)' }}>
      <span>{message}</span>
      {onRetry && <button type="button" onClick={onRetry} style={{ marginLeft: 12 }}>Retry</button>}
    </div>
  )
}
```

`app/src/auth/AuthContext.tsx`:

```tsx
import { useQueryClient } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { login as apiLogin, logout as apiLogout, me } from '../api/auth'
import { refreshSession, tokens } from '../api/client'
import type { StaffProfile } from '../api/types'

type Status = 'loading' | 'anonymous' | 'authenticated'

export interface AuthValue {
  status: Status
  staff: StaffProfile | null
  isAdmin: boolean
  departmentIds: number[]
  notice: string | null
  login(username: string, password: string): Promise<void>
  logout(): Promise<void>
}

const AuthContext = createContext<AuthValue | null>(null)
const EXPIRED = 'Your session expired. Please sign in again.'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<Status>('loading')
  const [staff, setStaff] = useState<StaffProfile | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const queryClient = useQueryClient()

  useEffect(() => {
    let cancelled = false
    tokens.setOnSessionLost(() => {
      if (cancelled) return
      setStaff(null)
      setStatus('anonymous')
      setNotice(EXPIRED)
      queryClient.clear()
    })
    ;(async () => {
      if (!tokens.getRefresh()) { if (!cancelled) setStatus('anonymous'); return }
      const ok = await refreshSession()
      if (cancelled) return
      if (!ok) { setStatus('anonymous'); setNotice(EXPIRED); return }
      try {
        const profile = await me()
        if (cancelled) return
        setStaff(profile)
        setStatus('authenticated')
      } catch {
        if (!cancelled) { tokens.clear(); setStatus('anonymous'); setNotice(EXPIRED) }
      }
    })()
    return () => { cancelled = true; tokens.setOnSessionLost(null) }
  }, [queryClient])

  const login = useCallback(async (username: string, password: string) => {
    const s = await apiLogin(username, password)
    setStaff(s.staff)
    setNotice(null)
    setStatus('authenticated')
  }, [])

  const logout = useCallback(async () => {
    await apiLogout()
    setStaff(null)
    setStatus('anonymous')
    setNotice(null)
    queryClient.clear()
  }, [queryClient])

  const value = useMemo<AuthValue>(() => ({
    status, staff, notice, login, logout,
    isAdmin: staff?.is_admin ?? false,
    departmentIds: staff?.department_ids ?? [],
  }), [status, staff, notice, login, logout])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthValue {
  const v = useContext(AuthContext)
  if (!v) throw new Error('useAuth must be used inside AuthProvider')
  return v
}
```

`app/src/auth/RequireAuth.tsx`:

```tsx
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { LoadingScreen } from '../components/LoadingScreen'
import { useAuth } from './AuthContext'

export function RequireAuth() {
  const { status } = useAuth()
  const location = useLocation()
  if (status === 'loading') return <LoadingScreen />
  if (status === 'anonymous') return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />
  return <Outlet />
}
```

`app/src/components/Layout.tsx`:

```tsx
import { Link, NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import { staffName } from '../lib/format'
import styles from './Layout.module.css'

export function Layout() {
  const { staff, logout } = useAuth()
  return (
    <div className={styles.shell}>
      <header className={styles.header}>
        <Link to="/tickets" className={styles.brand}>Ticket Desk</Link>
        <nav className={styles.nav}>
          <NavLink to="/tickets" end>Tickets</NavLink>
          <NavLink to="/tickets/new">New ticket</NavLink>
        </nav>
        <div className={styles.user}>
          {staff && <span>{staffName(staff)}</span>}
          <button type="button" onClick={() => void logout()}>Sign out</button>
        </div>
      </header>
      <main className={styles.main}>
        <Outlet />
      </main>
    </div>
  )
}
```

`app/src/components/Layout.module.css`:

```css
.shell { min-height: 100vh; display: flex; flex-direction: column; }
.header { display: flex; align-items: center; gap: 24px; padding: 10px 20px; background: var(--panel); border-bottom: 1px solid var(--border); }
.brand { font-weight: 700; color: var(--text); }
.nav { display: flex; gap: 16px; }
.nav :global(a.active) { font-weight: 600; }
.user { margin-left: auto; display: flex; gap: 12px; align-items: center; }
.main { padding: 20px; max-width: 1200px; width: 100%; margin: 0 auto; }
```

`app/src/pages/LoginPage.tsx`:

```tsx
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
```

`app/src/pages/NotFoundPage.tsx`:

```tsx
import { Link } from 'react-router-dom'

export function NotFoundPage() {
  return (
    <div className="panel">
      <h1>Page not found</h1>
      <Link to="/tickets">Back to tickets</Link>
    </div>
  )
}
```

`app/src/App.tsx`:

```tsx
import { Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import { RequireAuth } from './auth/RequireAuth'
import { Layout } from './components/Layout'
import { LoginPage } from './pages/LoginPage'
import { NotFoundPage } from './pages/NotFoundPage'

function TicketsPlaceholder() { return <h1>Tickets</h1> }

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<RequireAuth />}>
          <Route element={<Layout />}>
            <Route path="/" element={<Navigate to="/tickets" replace />} />
            <Route path="/tickets" element={<TicketsPlaceholder />} />
            <Route path="/tickets/new" element={<TicketsPlaceholder />} />
            <Route path="/tickets/:id" element={<TicketsPlaceholder />} />
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Route>
      </Routes>
    </AuthProvider>
  )
}
```

`app/src/main.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import './styles/global.css'

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 10_000 } } })

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
```

The placeholder `/tickets` page in this task must trigger a `GET /tickets` request so the mid-session refresh-failure test has something to fail; replace `TicketsPlaceholder` with:

```tsx
import { useQuery } from '@tanstack/react-query'
import { listTickets } from './api/tickets'

function TicketsPlaceholder() {
  const q = useQuery({ queryKey: ['tickets', { page: 1, page_size: 25 }], queryFn: () => listTickets({ page: 1, page_size: 25 }) })
  return <h1>Tickets{q.data ? ` (${q.data.total})` : ''}</h1>
}
```

(Task 5 replaces it with the real page.)

- [ ] **Step 4: Run tests, lint, build**

Run: `npm test && npm run lint && npm run build`
Expected: PASS (7 auth/app tests plus the 16 from earlier).

- [ ] **Step 5: Commit**

```bash
git add app/src
git commit -m "feat(app): auth provider, route guard, login page and app shell" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: Ticket list page with URL-bound filters

**Files:**
- Create: `app/src/hooks/useReferenceData.ts`, `app/src/hooks/useDebouncedValue.ts`, `app/src/hooks/useTicketFilters.ts`, `app/src/components/StatusBadge.tsx`, `app/src/components/Pagination.tsx`, `app/src/components/FilterBar.tsx`, `app/src/components/TicketRow.tsx`, `app/src/pages/TicketListPage.tsx`, `app/src/pages/TicketListPage.module.css`, `app/src/hooks/useTicketFilters.test.tsx`, `app/src/pages/TicketListPage.test.tsx`
- Modify: `app/src/App.tsx` (route `/tickets` → `TicketListPage`; remove `TicketsPlaceholder`)

**Interfaces:**
- Consumes: `listTickets`, reference list functions, `useAuth`, `formatDateTime`, `ErrorBanner`, `LoadingScreen`.
- Produces: `useReferenceData()` returning `{priorities, statuses, departments, topics, staff, isLoading, error}` (arrays default to `[]`); `useDebouncedValue<T>(value, ms)`; `useTicketFilters()` returning `{filter: ListFilter, set(patch: Partial<ListFilter>): void}` where any change other than `page` resets `page` to 1; `StatusBadge({state, name})`; `Pagination({page, pageSize, total, onPage})`; `FilterBar({filter, onChange})`; `TicketRow({ticket})`; `TicketListPage`.

- [ ] **Step 1: Failing tests**

`app/src/hooks/useTicketFilters.test.tsx`:

```tsx
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
```

`app/src/pages/TicketListPage.test.tsx`:

```tsx
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../App'
import { REFRESH_KEY } from '../api/client'
import { ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'

let lastQuery = new URLSearchParams()
beforeEach(() => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  lastQuery = new URLSearchParams()
  server.use(http.get('/api/v1/tickets', ({ request }) => {
    lastQuery = new URL(request.url).searchParams
    const page = Number(lastQuery.get('page') ?? '1')
    return HttpResponse.json({ items: page === 1 ? [ticketFixture] : [{ ...ticketFixture, id: 8, number: '000008', subject: 'Second' }], page, page_size: 1, total: 2 })
  }))
})

test('renders rows and links to the detail page', async () => {
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByText('Printer on fire')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: /000007/ })).toHaveAttribute('href', '/tickets/7')
  expect(screen.getByText('Open')).toBeInTheDocument()
})

test('filters update the request query and the URL', async () => {
  renderWithProviders(<App />, { route: '/tickets' })
  await screen.findByText('Printer on fire')
  await userEvent.selectOptions(screen.getByLabelText(/state/i), 'closed')
  await waitFor(() => expect(lastQuery.get('state')).toBe('closed'))
  await userEvent.selectOptions(screen.getByLabelText(/assigned/i), 'me')
  await waitFor(() => expect(lastQuery.get('assigned_to')).toBe('me'))
  await userEvent.type(screen.getByLabelText(/search/i), 'printer')
  await waitFor(() => expect(lastQuery.get('q')).toBe('printer'), { timeout: 2000 })
  expect(lastQuery.get('page')).toBe('1')
})

test('sort header toggles direction', async () => {
  renderWithProviders(<App />, { route: '/tickets' })
  await screen.findByText('Printer on fire')
  await userEvent.click(screen.getByRole('button', { name: /priority/i }))
  await waitFor(() => expect(lastQuery.get('sort')).toBe('priority'))
  await userEvent.click(screen.getByRole('button', { name: /priority/i }))
  await waitFor(() => expect(lastQuery.get('sort')).toBe('-priority'))
})

test('pagination requests the next page', async () => {
  renderWithProviders(<App />, { route: '/tickets?page_size=1' })
  await screen.findByText('Printer on fire')
  await userEvent.click(screen.getByRole('button', { name: /next/i }))
  expect(await screen.findByText('Second')).toBeInTheDocument()
  expect(lastQuery.get('page')).toBe('2')
})

test('shows the error banner with retry', async () => {
  server.use(http.get('/api/v1/tickets', () => HttpResponse.json({ error: { code: 'internal', message: 'internal server error' } }, { status: 500 })))
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByRole('alert')).toHaveTextContent(/internal server error/i)
  expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npm test -- src/hooks src/pages/TicketListPage`
Expected: FAIL, modules not found.

- [ ] **Step 3: Implement hooks**

`app/src/hooks/useReferenceData.ts`:

```ts
import { useQueries } from '@tanstack/react-query'
import { listDepartments, listPriorities, listStaff, listStatuses, listTopics } from '../api/reference'

export function useReferenceData() {
  const [priorities, statuses, departments, topics, staff] = useQueries({
    queries: [
      { queryKey: ['ref', 'priorities'], queryFn: listPriorities, staleTime: 5 * 60_000 },
      { queryKey: ['ref', 'statuses'], queryFn: listStatuses, staleTime: 5 * 60_000 },
      { queryKey: ['ref', 'departments'], queryFn: listDepartments, staleTime: 5 * 60_000 },
      { queryKey: ['ref', 'topics'], queryFn: listTopics, staleTime: 5 * 60_000 },
      { queryKey: ['ref', 'staff'], queryFn: listStaff, staleTime: 60_000 },
    ],
  })
  return {
    priorities: priorities.data ?? [],
    statuses: statuses.data ?? [],
    departments: departments.data ?? [],
    topics: topics.data ?? [],
    staff: staff.data ?? [],
    isLoading: [priorities, statuses, departments, topics, staff].some((q) => q.isLoading),
    error: [priorities, statuses, departments, topics, staff].find((q) => q.error)?.error ?? null,
  }
}
```

`app/src/hooks/useDebouncedValue.ts`:

```ts
import { useEffect, useState } from 'react'

export function useDebouncedValue<T>(value: T, ms: number): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return debounced
}
```

`app/src/hooks/useTicketFilters.ts`:

```ts
import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { ListFilter, TicketState } from '../api/types'

const STATES: TicketState[] = ['open', 'resolved', 'closed']
export const SORTS = ['created_at', '-created_at', 'last_message_at', '-last_message_at', 'priority', '-priority'] as const
export const DEFAULT_SORT = '-last_message_at'
const DEFAULT_PAGE_SIZE = 25

function posInt(v: string | null, max = Number.MAX_SAFE_INTEGER): number | undefined {
  if (!v || !/^\d+$/.test(v)) return undefined
  const n = Number(v)
  return n >= 1 && n <= max ? n : undefined
}

export function parseFilter(params: URLSearchParams): ListFilter {
  const state = params.get('state')
  const sort = params.get('sort')
  const assigned = params.get('assigned_to')
  const f: ListFilter = {
    page: posInt(params.get('page'), 1_000_000) ?? 1,
    page_size: posInt(params.get('page_size'), 100) ?? DEFAULT_PAGE_SIZE,
    sort: sort && (SORTS as readonly string[]).includes(sort) ? sort : DEFAULT_SORT,
  }
  if (state && (STATES as string[]).includes(state)) f.state = state as TicketState
  const status = posInt(params.get('status')); if (status) f.status = status
  const dept = posInt(params.get('dept_id')); if (dept) f.dept_id = dept
  if (assigned && (assigned === 'me' || assigned === 'none' || /^\d+$/.test(assigned))) f.assigned_to = assigned
  const q = params.get('q')?.trim(); if (q) f.q = q
  return f
}

export function useTicketFilters() {
  const [params, setParams] = useSearchParams()
  const filter = useMemo(() => parseFilter(params), [params])
  const set = useCallback((patch: Partial<ListFilter>) => {
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      const resetPage = Object.keys(patch).some((k) => k !== 'page')
      for (const [k, v] of Object.entries(patch)) {
        if (v === undefined || v === '' || v === null) next.delete(k)
        else next.set(k, String(v))
      }
      if (resetPage) next.delete('page')
      if (next.get('page') === '1') next.delete('page')
      return next
    })
  }, [setParams])
  return { filter, set }
}
```

- [ ] **Step 4: Implement components and page**

`app/src/components/StatusBadge.tsx`:

```tsx
import type { TicketState } from '../api/types'

const colors: Record<TicketState, string> = { open: 'var(--ok)', resolved: 'var(--warn)', closed: 'var(--muted)' }

export function StatusBadge({ state, name }: { state: TicketState; name: string }) {
  return <span style={{ color: colors[state], fontWeight: 600 }}>{name}</span>
}
```

`app/src/components/Pagination.tsx`:

```tsx
export function Pagination({ page, pageSize, total, onPage }: { page: number; pageSize: number; total: number; onPage: (p: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  return (
    <div className="row" style={{ justifyContent: 'flex-end', marginTop: 12 }}>
      <span className="muted">{total} tickets, page {page} of {pages}</span>
      <button type="button" disabled={page <= 1} onClick={() => onPage(page - 1)}>Previous</button>
      <button type="button" disabled={page >= pages} onClick={() => onPage(page + 1)}>Next</button>
    </div>
  )
}
```

`app/src/components/FilterBar.tsx`:

```tsx
import { useEffect, useState } from 'react'
import type { ListFilter } from '../api/types'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { useReferenceData } from '../hooks/useReferenceData'
import { staffName } from '../lib/format'

export function FilterBar({ filter, onChange }: { filter: ListFilter; onChange: (patch: Partial<ListFilter>) => void }) {
  const { statuses, departments, staff } = useReferenceData()
  const [q, setQ] = useState(filter.q ?? '')
  const debouncedQ = useDebouncedValue(q, 300)
  useEffect(() => { if ((filter.q ?? '') !== debouncedQ) onChange({ q: debouncedQ || undefined }) }, [debouncedQ]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="row panel">
      <label>State <select aria-label="State" value={filter.state ?? ''} onChange={(e) => onChange({ state: (e.target.value || undefined) as ListFilter['state'] })}>
        <option value="">All</option><option value="open">Open</option><option value="resolved">Resolved</option><option value="closed">Closed</option>
      </select></label>
      <label>Status <select aria-label="Status" value={filter.status ?? ''} onChange={(e) => onChange({ status: e.target.value ? Number(e.target.value) : undefined })}>
        <option value="">Any</option>{statuses.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
      </select></label>
      <label>Department <select aria-label="Department" value={filter.dept_id ?? ''} onChange={(e) => onChange({ dept_id: e.target.value ? Number(e.target.value) : undefined })}>
        <option value="">Any</option>{departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
      </select></label>
      <label>Assigned <select aria-label="Assigned" value={filter.assigned_to ?? ''} onChange={(e) => onChange({ assigned_to: e.target.value || undefined })}>
        <option value="">All</option><option value="me">Me</option><option value="none">Unassigned</option>
        {staff.filter((s) => s.is_active).map((s) => <option key={s.id} value={s.id}>{staffName(s)}</option>)}
      </select></label>
      <label>Search <input aria-label="Search" placeholder="Subject" value={q} onChange={(e) => setQ(e.target.value)} /></label>
    </div>
  )
}
```

`app/src/components/TicketRow.tsx`:

```tsx
import { Link } from 'react-router-dom'
import type { Ticket } from '../api/types'
import { formatDateTime } from '../lib/format'
import { StatusBadge } from './StatusBadge'

export function TicketRow({ ticket: t }: { ticket: Ticket }) {
  return (
    <tr>
      <td><Link to={`/tickets/${t.id}`}>{t.number}</Link></td>
      <td><Link to={`/tickets/${t.id}`}>{t.subject}</Link></td>
      <td>{t.requester_name || t.requester_email}</td>
      <td>{t.department.name}</td>
      <td><StatusBadge state={t.state} name={t.status.name} /></td>
      <td>{t.priority.name}</td>
      <td>{t.assignee?.name ?? <span className="muted">Unassigned</span>}</td>
      <td>{formatDateTime(t.last_message_at)}</td>
    </tr>
  )
}
```

`app/src/pages/TicketListPage.tsx`:

```tsx
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { listTickets } from '../api/tickets'
import { ErrorBanner } from '../components/ErrorBanner'
import { FilterBar } from '../components/FilterBar'
import { LoadingScreen } from '../components/LoadingScreen'
import { Pagination } from '../components/Pagination'
import { TicketRow } from '../components/TicketRow'
import { useTicketFilters } from '../hooks/useTicketFilters'

export function TicketListPage() {
  const { filter, set } = useTicketFilters()
  const q = useQuery({ queryKey: ['tickets', filter], queryFn: () => listTickets(filter), placeholderData: keepPreviousData })

  function toggleSort(key: string) {
    set({ sort: filter.sort === key ? `-${key}` : key })
  }
  const sortLabel = (key: string, label: string) => {
    const dir = filter.sort === key ? ' ↑' : filter.sort === `-${key}` ? ' ↓' : ''
    return <button type="button" onClick={() => toggleSort(key)}>{label}{dir}</button>
  }

  return (
    <div>
      <div className="row" style={{ justifyContent: 'space-between', marginBottom: 12 }}>
        <h1 style={{ margin: 0 }}>Tickets</h1>
        <Link to="/tickets/new"><button type="button" className="primary">New ticket</button></Link>
      </div>
      <FilterBar filter={filter} onChange={set} />
      {q.isLoading && <LoadingScreen />}
      {q.error && <ErrorBanner error={q.error} onRetry={() => void q.refetch()} />}
      {q.data && (
        <>
          <table>
            <thead>
              <tr>
                <th>Number</th><th>Subject</th><th>Requester</th><th>Department</th><th>Status</th>
                <th>{sortLabel('priority', 'Priority')}</th><th>Assignee</th><th>{sortLabel('last_message_at', 'Last message')}</th>
              </tr>
            </thead>
            <tbody>
              {q.data.items.length === 0 && <tr><td colSpan={8} className="muted">No tickets match.</td></tr>}
              {q.data.items.map((t) => <TicketRow key={t.id} ticket={t} />)}
            </tbody>
          </table>
          <Pagination page={q.data.page} pageSize={q.data.page_size} total={q.data.total} onPage={(p) => set({ page: p })} />
        </>
      )}
    </div>
  )
}
```

In `app/src/App.tsx`, import `TicketListPage` and use it for `/tickets`; keep the placeholder only for `/tickets/new` and `/tickets/:id` until Tasks 6 and 9.

- [ ] **Step 5: Run tests, lint, build**

Run: `npm test && npm run lint && npm run build`
Expected: PASS (8 new tests).

- [ ] **Step 6: Commit**

```bash
git add app/src
git commit -m "feat(app): ticket list page with URL-bound filters, sorting and pagination" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: Ticket detail page and header actions

**Files:**
- Create: `app/src/pages/TicketDetailPage.tsx`, `app/src/components/TicketHeader.tsx`, `app/src/components/TicketHeader.module.css`, `app/src/hooks/useTicketMutations.ts`, `app/src/pages/TicketDetailPage.test.tsx`
- Modify: `app/src/App.tsx` (route `/tickets/:id` → `TicketDetailPage`)

**Interfaces:**
- Consumes: `getTicket`, `setStatus`, `assign`, `transfer`, `updateTicket` from `api/tickets.ts`; `useReferenceData`; `useAuth`; `StatusBadge`, `ErrorBanner`, `LoadingScreen`, `formatDateTime`, `staffName`.
- Produces: `useTicketMutations(id)` returning `{setStatus, assign, transfer, update}` `useMutation` results whose `onSuccess` invalidates the four keys; `TicketHeader({ticket})`; `TicketDetailPage` with placeholder sections `<section aria-label="Thread"/>` and `<section aria-label="Composer"/>` that Tasks 7 and 8 fill; `ticketQueryKey(id) => ['ticket', id]`.

- [ ] **Step 1: Failing tests**

`app/src/pages/TicketDetailPage.test.tsx`:

```tsx
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../App'
import { REFRESH_KEY } from '../api/client'
import { ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'

beforeEach(() => localStorage.setItem(REFRESH_KEY, 'refresh-1'))

test('renders header fields', async () => {
  renderWithProviders(<App />, { route: '/tickets/7' })
  expect(await screen.findByRole('heading', { name: /000007/ })).toHaveTextContent('Printer on fire')
  const header = screen.getByRole('region', { name: /ticket header/i })
  expect(within(header).getByText('Support')).toBeInTheDocument()
  expect(within(header).getByText('General Inquiry')).toBeInTheDocument()
  expect(within(header).getByText(/pat@example.test/)).toBeInTheDocument()
  expect(within(header).getByText('Unassigned')).toBeInTheDocument()
})

test('404 renders not found', async () => {
  renderWithProviders(<App />, { route: '/tickets/999' })
  expect(await screen.findByText(/ticket not found/i)).toBeInTheDocument()
})

test('status change posts and re-renders', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets/7/status', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ ...ticketFixture, status: { id: 3, name: 'Closed' }, state: 'closed', closed_at: '2026-09-25T11:00:00Z' })
  }))
  let getCalls = 0
  server.use(http.get('/api/v1/tickets/7', () => { getCalls++; return HttpResponse.json(getCalls > 1 ? { ...ticketFixture, status: { id: 3, name: 'Closed' }, state: 'closed' } : ticketFixture) }))
  renderWithProviders(<App />, { route: '/tickets/7' })
  await screen.findByRole('heading', { name: /000007/ })
  await userEvent.selectOptions(await screen.findByLabelText(/^status$/i), '3')
  await waitFor(() => expect(body).toEqual({ status_id: 3 }))
  const header = screen.getByRole('region', { name: /ticket header/i })
  await waitFor(() => expect(within(header).getByText('Closed')).toBeInTheDocument())
})

test('assign lists only staff who can see the department, and posts', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets/7/assign', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...ticketFixture, assignee: { id: 1, name: 'Ann Agent' } }) }))
  renderWithProviders(<App />, { route: '/tickets/7' })
  await screen.findByRole('heading', { name: /000007/ })
  const select = await screen.findByLabelText(/^assignee$/i)
  const options = within(select).getAllByRole('option').map((o) => o.textContent)
  expect(options).toEqual(['Unassigned', 'Ann Agent', 'Root Admin'])
  await userEvent.selectOptions(select, '1')
  await waitFor(() => expect(body).toEqual({ staff_id: 1 }))
})

test('transfer posts the department and shows a validation error on 400', async () => {
  server.use(http.post('/api/v1/tickets/7/transfer', () =>
    HttpResponse.json({ error: { code: 'forbidden', message: 'forbidden: cannot transfer to that department' } }, { status: 403 })))
  renderWithProviders(<App />, { route: '/tickets/7' })
  await screen.findByRole('heading', { name: /000007/ })
  await userEvent.selectOptions(await screen.findByLabelText(/^department$/i), '2')
  expect(await screen.findByRole('alert')).toHaveTextContent(/cannot transfer/i)
})

test('edit form patches subject and priority', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/tickets/7', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...ticketFixture, subject: 'New subject' }) }))
  renderWithProviders(<App />, { route: '/tickets/7' })
  await screen.findByRole('heading', { name: /000007/ })
  await userEvent.click(screen.getByRole('button', { name: /edit/i }))
  const subject = screen.getByLabelText(/subject/i)
  await userEvent.clear(subject)
  await userEvent.type(subject, 'New subject')
  await userEvent.selectOptions(screen.getByLabelText(/^priority$/i), '3')
  await userEvent.click(screen.getByRole('button', { name: /save/i }))
  await waitFor(() => expect(body).toMatchObject({ subject: 'New subject', priority_id: 3 }))
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npm test -- src/pages/TicketDetailPage`
Expected: FAIL, modules not found.

- [ ] **Step 3: Implement mutations hook**

`app/src/hooks/useTicketMutations.ts`:

```ts
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { assign, setStatus, transfer, updateTicket } from '../api/tickets'
import type { UpdateTicketInput } from '../api/types'

export const ticketQueryKey = (id: number) => ['ticket', id] as const

export function useInvalidateTicket(id: number) {
  const qc = useQueryClient()
  return async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['ticket', id] }),
      qc.invalidateQueries({ queryKey: ['thread', id] }),
      qc.invalidateQueries({ queryKey: ['events', id] }),
      qc.invalidateQueries({ queryKey: ['tickets'] }),
    ])
  }
}

export function useTicketMutations(id: number) {
  const invalidate = useInvalidateTicket(id)
  return {
    setStatus: useMutation({ mutationFn: (statusId: number) => setStatus(id, statusId), onSuccess: invalidate }),
    assign: useMutation({ mutationFn: (staffId: number | null) => assign(id, staffId), onSuccess: invalidate }),
    transfer: useMutation({ mutationFn: (deptId: number) => transfer(id, deptId), onSuccess: invalidate }),
    update: useMutation({ mutationFn: (input: UpdateTicketInput) => updateTicket(id, input), onSuccess: invalidate }),
  }
}
```

- [ ] **Step 4: Implement the header**

`app/src/components/TicketHeader.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { ApiError } from '../api/client'
import type { Ticket } from '../api/types'
import { useAuth } from '../auth/AuthContext'
import { useReferenceData } from '../hooks/useReferenceData'
import { useTicketMutations } from '../hooks/useTicketMutations'
import { formatDateTime, staffName } from '../lib/format'
import { ErrorBanner } from './ErrorBanner'
import { StatusBadge } from './StatusBadge'
import styles from './TicketHeader.module.css'

export function TicketHeader({ ticket }: { ticket: Ticket }) {
  const { isAdmin, departmentIds } = useAuth()
  const { statuses, departments, staff, priorities, topics } = useReferenceData()
  const m = useTicketMutations(ticket.id)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState({
    subject: ticket.subject, priority_id: ticket.priority.id, topic_id: ticket.topic?.id ?? 0,
    due_at: ticket.due_at ? ticket.due_at.slice(0, 16) : '', requester_name: ticket.requester_name, requester_email: ticket.requester_email,
  })
  const error = m.setStatus.error ?? m.assign.error ?? m.transfer.error ?? m.update.error
  const fields = error instanceof ApiError ? error.fields : {}
  const busy = m.setStatus.isPending || m.assign.isPending || m.transfer.isPending || m.update.isPending

  const assignable = staff.filter((s) => s.is_active && (s.is_admin || s.department_ids.includes(ticket.department.id)))
  const visibleDepts = departments.filter((d) => isAdmin || departmentIds.includes(d.id))

  function save(e: FormEvent) {
    e.preventDefault()
    m.update.mutate({
      subject: form.subject, priority_id: form.priority_id, topic_id: form.topic_id || undefined,
      due_at: form.due_at ? new Date(form.due_at).toISOString() : undefined,
      requester_name: form.requester_name, requester_email: form.requester_email,
    }, { onSuccess: () => setEditing(false) })
  }

  return (
    <section aria-label="Ticket header" className="panel">
      <h1 className={styles.title}>{ticket.number} · {ticket.subject}</h1>
      {error && <ErrorBanner error={error} />}
      <dl className={styles.grid}>
        <dt>Requester</dt><dd>{ticket.requester_name} &lt;{ticket.requester_email}&gt;</dd>
        <dt>Status</dt><dd>
          <StatusBadge state={ticket.state} name={ticket.status.name} />{' '}
          <select aria-label="Status" value={ticket.status.id} disabled={busy} onChange={(e) => m.setStatus.mutate(Number(e.target.value))}>
            {statuses.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
        </dd>
        <dt>Department</dt><dd>
          <span>{ticket.department.name}</span>{' '}
          <select aria-label="Department" value={ticket.department.id} disabled={busy} onChange={(e) => m.transfer.mutate(Number(e.target.value))}>
            {visibleDepts.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </select>
        </dd>
        <dt>Assignee</dt><dd>
          <span>{ticket.assignee?.name ?? 'Unassigned'}</span>{' '}
          <select aria-label="Assignee" value={ticket.assignee?.id ?? ''} disabled={busy} onChange={(e) => m.assign.mutate(e.target.value ? Number(e.target.value) : null)}>
            <option value="">Unassigned</option>
            {assignable.map((s) => <option key={s.id} value={s.id}>{staffName(s)}</option>)}
          </select>
        </dd>
        <dt>Topic</dt><dd>{ticket.topic?.name ?? <span className="muted">None</span>}</dd>
        <dt>Priority</dt><dd>{ticket.priority.name}</dd>
        <dt>Created</dt><dd>{formatDateTime(ticket.created_at)}</dd>
        <dt>Due</dt><dd>{ticket.due_at ? formatDateTime(ticket.due_at) : <span className="muted">None</span>}</dd>
        {ticket.closed_at && <><dt>Closed</dt><dd>{formatDateTime(ticket.closed_at)}</dd></>}
      </dl>
      {!editing && <button type="button" onClick={() => setEditing(true)}>Edit</button>}
      {editing && (
        <form onSubmit={save} className={styles.editForm}>
          <div className="field"><label htmlFor="subject">Subject</label>
            <input id="subject" value={form.subject} onChange={(e) => setForm({ ...form, subject: e.target.value })} required />
            {fields.subject && <span className="field-error">{fields.subject}</span>}</div>
          <div className="field"><label htmlFor="priority">Priority</label>
            <select id="priority" value={form.priority_id} onChange={(e) => setForm({ ...form, priority_id: Number(e.target.value) })}>
              {priorities.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></div>
          <div className="field"><label htmlFor="topic">Topic</label>
            <select id="topic" value={form.topic_id} onChange={(e) => setForm({ ...form, topic_id: Number(e.target.value) })}>
              <option value={0}>None</option>{topics.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</select></div>
          <div className="field"><label htmlFor="due">Due</label>
            <input id="due" type="datetime-local" value={form.due_at} onChange={(e) => setForm({ ...form, due_at: e.target.value })} /></div>
          <div className="field"><label htmlFor="rname">Requester name</label>
            <input id="rname" value={form.requester_name} onChange={(e) => setForm({ ...form, requester_name: e.target.value })} /></div>
          <div className="field"><label htmlFor="remail">Requester email</label>
            <input id="remail" type="email" value={form.requester_email} onChange={(e) => setForm({ ...form, requester_email: e.target.value })} required />
            {fields.requester_email && <span className="field-error">{fields.requester_email}</span>}</div>
          <div className="row">
            <button type="submit" className="primary" disabled={busy}>Save</button>
            <button type="button" onClick={() => setEditing(false)}>Cancel</button>
          </div>
        </form>
      )}
    </section>
  )
}
```

`app/src/components/TicketHeader.module.css`:

```css
.title { margin: 0 0 12px; font-size: 20px; }
.grid { display: grid; grid-template-columns: 120px 1fr; gap: 6px 12px; margin: 0 0 12px; }
.grid dt { color: var(--muted); }
.grid dd { margin: 0; }
.editForm { margin-top: 12px; max-width: 480px; }
```

- [ ] **Step 5: Implement the page**

`app/src/pages/TicketDetailPage.tsx`:

```tsx
import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { getTicket } from '../api/tickets'
import { ErrorBanner } from '../components/ErrorBanner'
import { LoadingScreen } from '../components/LoadingScreen'
import { TicketHeader } from '../components/TicketHeader'
import { ticketQueryKey } from '../hooks/useTicketMutations'

export function TicketDetailPage() {
  const params = useParams()
  const id = Number(params.id)
  const q = useQuery({ queryKey: ticketQueryKey(id), queryFn: () => getTicket(id), enabled: Number.isInteger(id) && id > 0 })

  if (!Number.isInteger(id) || id <= 0) return <NotFound />
  if (q.isLoading) return <LoadingScreen />
  if (q.error instanceof ApiError && q.error.status === 404) return <NotFound />
  if (q.error) return <ErrorBanner error={q.error} onRetry={() => void q.refetch()} />
  if (!q.data) return null

  return (
    <div>
      <p><Link to="/tickets">← Tickets</Link></p>
      <TicketHeader ticket={q.data} />
      <section aria-label="Thread" className="panel" />
      <section aria-label="Composer" className="panel" />
    </div>
  )
}

function NotFound() {
  return (
    <div className="panel">
      <h1>Ticket not found</h1>
      <Link to="/tickets">Back to tickets</Link>
    </div>
  )
}
```

Wire `/tickets/:id` to `TicketDetailPage` in `App.tsx`.

- [ ] **Step 6: Run tests, lint, build**

Run: `npm test && npm run lint && npm run build`
Expected: PASS (6 new tests).

- [ ] **Step 7: Commit**

```bash
git add app/src
git commit -m "feat(app): ticket detail page with status, assign, transfer and edit actions" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: Thread, entries, attachments, events

**Files:**
- Create: `app/src/components/ThreadEntry.tsx`, `app/src/components/ThreadEntry.module.css`, `app/src/components/AttachmentList.tsx`, `app/src/components/Thread.tsx`, `app/src/components/EventsPanel.tsx`, `app/src/components/Thread.test.tsx`
- Modify: `app/src/pages/TicketDetailPage.tsx` (replace the Thread placeholder with `<Thread ticketId={id}/>` and add `<EventsPanel ticketId={id}/>`)

**Interfaces:**
- Consumes: `getThread`, `getEvents`, `downloadFile`, `formatDateTime`.
- Produces: `Thread({ticketId})` using `useInfiniteQuery` with key `['thread', ticketId]`; `ThreadEntry({entry})`; `AttachmentList({attachments})`; `EventsPanel({ticketId})` with key `['events', ticketId]`.

- [ ] **Step 1: Failing tests**

`app/src/components/Thread.test.tsx`:

```tsx
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { REFRESH_KEY, tokens } from '../api/client'
import { sessionFixture, entryFixtures } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'
import { EventsPanel } from './EventsPanel'
import { Thread } from './Thread'

beforeEach(() => { localStorage.setItem(REFRESH_KEY, 'refresh-1'); tokens.setSession(sessionFixture) })

test('renders entries oldest first with type badges and attachments', async () => {
  renderWithProviders(<Thread ticketId={7} />)
  const items = await screen.findAllByRole('article')
  expect(items).toHaveLength(2)
  expect(within(items[0]!).getByText('Message')).toBeInTheDocument()
  expect(within(items[0]!).getByText('Pat')).toBeInTheDocument()
  expect(within(items[1]!).getByText('Response')).toBeInTheDocument()
  expect(within(items[1]!).getByRole('button', { name: /log\.txt/ })).toBeInTheDocument()
})

test('sanitises HTML bodies', async () => {
  server.use(http.get('/api/v1/tickets/7/thread', () => HttpResponse.json({
    items: [{ ...entryFixtures[0], body: '<p>hi</p><script>window.__pwned=1</script><img src=x onerror="window.__pwned=2">' }], next_after: null,
  })))
  renderWithProviders(<Thread ticketId={7} />)
  const item = await screen.findByRole('article')
  expect(item.querySelector('script')).toBeNull()
  expect(item.querySelector('img')?.getAttribute('onerror')).toBeNull()
  expect(item).toHaveTextContent('hi')
  expect((window as unknown as { __pwned?: number }).__pwned).toBeUndefined()
})

test('text bodies keep line breaks and are not parsed as HTML', async () => {
  server.use(http.get('/api/v1/tickets/7/thread', () => HttpResponse.json({
    items: [{ ...entryFixtures[1], body: 'line one\n<b>not bold</b>' }], next_after: null,
  })))
  renderWithProviders(<Thread ticketId={7} />)
  const item = await screen.findByRole('article')
  expect(item.querySelector('b')).toBeNull()
  expect(item).toHaveTextContent('<b>not bold</b>')
})

test('load more appends the next page', async () => {
  server.use(http.get('/api/v1/tickets/7/thread', ({ request }) => {
    const after = new URL(request.url).searchParams.get('after')
    if (!after) return HttpResponse.json({ items: [entryFixtures[0]], next_after: 1 })
    return HttpResponse.json({ items: [entryFixtures[1]], next_after: null })
  }))
  renderWithProviders(<Thread ticketId={7} />)
  expect(await screen.findAllByRole('article')).toHaveLength(1)
  await userEvent.click(screen.getByRole('button', { name: /load more/i }))
  expect(await screen.findAllByRole('article')).toHaveLength(2)
  expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument()
})

test('attachment click downloads with auth', async () => {
  const createObjectURL = vi.fn(() => 'blob:x')
  Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() })
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  renderWithProviders(<Thread ticketId={7} />)
  await userEvent.click(await screen.findByRole('button', { name: /log\.txt/ }))
  await vi.waitFor(() => expect(click).toHaveBeenCalledTimes(1))
  click.mockRestore()
})

test('events panel lists the audit trail when expanded', async () => {
  renderWithProviders(<EventsPanel ticketId={7} />)
  await userEvent.click(screen.getByRole('button', { name: /history/i }))
  expect(await screen.findByText(/created/i)).toBeInTheDocument()
  expect(screen.getByText(/Ann Agent/)).toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npm test -- src/components/Thread`
Expected: FAIL, modules not found.

- [ ] **Step 3: Implement**

`app/src/components/AttachmentList.tsx`:

```tsx
import { useState } from 'react'
import { downloadFile } from '../api/files'
import type { AttachmentRef } from '../api/types'

function size(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export function AttachmentList({ attachments }: { attachments: AttachmentRef[] }) {
  const [error, setError] = useState<string | null>(null)
  if (attachments.length === 0) return null
  return (
    <ul className="row" style={{ listStyle: 'none', padding: 0, margin: '8px 0 0' }}>
      {attachments.map((a) => (
        <li key={a.file_id}>
          <button type="button" onClick={() => downloadFile(a.file_id, a.name).catch(() => setError(`Could not download ${a.name}`))}>
            {a.name} <span className="muted">({size(a.size)})</span>
          </button>
        </li>
      ))}
      {error && <li role="alert" className="field-error">{error}</li>}
    </ul>
  )
}
```

`app/src/components/ThreadEntry.tsx`:

```tsx
import DOMPurify from 'dompurify'
import type { Entry } from '../api/types'
import { formatDateTime } from '../lib/format'
import { AttachmentList } from './AttachmentList'
import styles from './ThreadEntry.module.css'

const labels: Record<Entry['type'], string> = { message: 'Message', response: 'Response', note: 'Note' }

export function ThreadEntry({ entry }: { entry: Entry }) {
  return (
    <article className={`${styles.entry} ${styles[entry.type]}`}>
      <header className={styles.head}>
        <span className={styles.badge}>{labels[entry.type]}</span>
        <strong>{entry.poster || 'Unknown'}</strong>
        <span className="muted">{formatDateTime(entry.created_at)}</span>
        {entry.title && <em>{entry.title}</em>}
      </header>
      {entry.format === 'html'
        ? <div dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(entry.body, { USE_PROFILES: { html: true } }) }} />
        : <pre className="pre">{entry.body}</pre>}
      <AttachmentList attachments={entry.attachments} />
    </article>
  )
}
```

`app/src/components/ThreadEntry.module.css`:

```css
.entry { border: 1px solid var(--border); border-radius: var(--radius); padding: 12px; margin-bottom: 10px; background: var(--panel); }
.note { background: #fffbe6; }
.response { border-left: 3px solid var(--accent); }
.head { display: flex; gap: 10px; align-items: baseline; margin-bottom: 8px; }
.badge { font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); }
```

`app/src/components/Thread.tsx`:

```tsx
import { useInfiniteQuery } from '@tanstack/react-query'
import { getThread } from '../api/tickets'
import { ErrorBanner } from './ErrorBanner'
import { LoadingScreen } from './LoadingScreen'
import { ThreadEntry } from './ThreadEntry'

export function Thread({ ticketId }: { ticketId: number }) {
  const q = useInfiniteQuery({
    queryKey: ['thread', ticketId],
    queryFn: ({ pageParam }) => getThread(ticketId, pageParam),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_after ?? undefined,
  })
  if (q.isLoading) return <LoadingScreen label="Loading thread…" />
  if (q.error) return <ErrorBanner error={q.error} onRetry={() => void q.refetch()} />
  const entries = q.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <section aria-label="Thread">
      <h2>Thread</h2>
      {entries.length === 0 && <p className="muted">No entries yet.</p>}
      {entries.map((e) => <ThreadEntry key={e.id} entry={e} />)}
      {q.hasNextPage && <button type="button" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>Load more</button>}
    </section>
  )
}
```

`app/src/components/EventsPanel.tsx`:

```tsx
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { getEvents } from '../api/tickets'
import { formatDateTime } from '../lib/format'
import { ErrorBanner } from './ErrorBanner'

export function EventsPanel({ ticketId }: { ticketId: number }) {
  const [open, setOpen] = useState(false)
  const q = useQuery({ queryKey: ['events', ticketId], queryFn: () => getEvents(ticketId), enabled: open })
  return (
    <section aria-label="History" className="panel">
      <button type="button" onClick={() => setOpen((o) => !o)}>{open ? 'Hide history' : 'Show history'}</button>
      {open && q.error && <ErrorBanner error={q.error} onRetry={() => void q.refetch()} />}
      {open && q.data && (
        <ul>
          {q.data.map((ev) => (
            <li key={ev.id}>
              <span className="muted">{formatDateTime(ev.created_at)}</span>{' '}
              <strong>{ev.kind.replace('_', ' ')}</strong>
              {ev.staff && <> by {ev.staff.name || `staff #${ev.staff.id}`}</>}
              {Object.keys(ev.data).length > 0 && <span className="muted"> {JSON.stringify(ev.data)}</span>}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
```

In `TicketDetailPage.tsx`, replace `<section aria-label="Thread" className="panel" />` with `<div className="panel"><Thread ticketId={id} /></div>` and add `<EventsPanel ticketId={id} />` after the composer placeholder.

- [ ] **Step 4: Run tests, lint, build**

Run: `npm test && npm run lint && npm run build`
Expected: PASS (6 new tests).

- [ ] **Step 5: Commit**

```bash
git add app/src
git commit -m "feat(app): thread with sanitised entries, attachments download and events panel" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: Composer with reply, note and file upload

**Files:**
- Create: `app/src/components/FileUpload.tsx`, `app/src/components/Composer.tsx`, `app/src/components/Composer.module.css`, `app/src/components/Composer.test.tsx`
- Modify: `app/src/pages/TicketDetailPage.tsx` (replace the Composer placeholder with `<Composer ticketId={id} />`)

**Interfaces:**
- Consumes: `reply`, `note` from `api/tickets.ts`; `uploadFile` from `api/files.ts`; `useReferenceData`; `useInvalidateTicket`; `ApiError`.
- Produces: `FileUpload({pending, onChange})` where `pending: PendingFile[]` and `PendingFile = {key: string; name: string; status: 'uploading'|'done'|'error'; fileId?: number; error?: string}`; `Composer({ticketId})`.

- [ ] **Step 1: Failing tests**

`app/src/components/Composer.test.tsx`:

```tsx
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse, delay } from 'msw'
import { REFRESH_KEY, tokens } from '../api/client'
import { sessionFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'
import { Composer } from './Composer'

beforeEach(() => { localStorage.setItem(REFRESH_KEY, 'refresh-1'); tokens.setSession(sessionFixture) })

test('reply posts text body with optional status and invalidates', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets/7/reply', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9 }, { status: 201 }) }))
  const { client } = renderWithProviders(<Composer ticketId={7} />)
  const spy = vi.spyOn(client, 'invalidateQueries')
  await userEvent.type(await screen.findByLabelText(/^reply$/i), 'Thanks, fixed.')
  await userEvent.selectOptions(screen.getByLabelText(/set status/i), '3')
  await userEvent.click(screen.getByRole('button', { name: /send reply/i }))
  await waitFor(() => expect(body).toEqual({ body: 'Thanks, fixed.', format: 'text', status_id: 3, file_ids: [] }))
  await waitFor(() => expect(spy).toHaveBeenCalledWith({ queryKey: ['thread', 7] }))
  expect(screen.getByLabelText(/^reply$/i)).toHaveValue('')
})

test('note tab posts to /notes with a title', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets/7/notes', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 10 }, { status: 201 }) }))
  renderWithProviders(<Composer ticketId={7} />)
  await userEvent.click(screen.getByRole('tab', { name: /internal note/i }))
  await userEvent.type(screen.getByLabelText(/title/i), 'Checked logs')
  await userEvent.type(screen.getByLabelText(/^note$/i), 'Nothing unusual')
  await userEvent.click(screen.getByRole('button', { name: /add note/i }))
  await waitFor(() => expect(body).toEqual({ title: 'Checked logs', body: 'Nothing unusual', format: 'text', file_ids: [] }))
})

test('validation error from the API shows on the field', async () => {
  server.use(http.post('/api/v1/tickets/7/reply', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { body: 'required' } } }, { status: 400 })))
  renderWithProviders(<Composer ticketId={7} />)
  await userEvent.type(screen.getByLabelText(/^reply$/i), 'x')
  await userEvent.click(screen.getByRole('button', { name: /send reply/i }))
  expect(await screen.findByText('required')).toBeInTheDocument()
})

test('upload adds a pending file and submit sends its id; submit is disabled while uploading', async () => {
  let replyBody: unknown
  server.use(
    http.post('/api/v1/files', async () => { await delay(150); return HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 }) }),
    http.post('/api/v1/tickets/7/reply', async ({ request }) => { replyBody = await request.json(); return HttpResponse.json({ id: 9 }, { status: 201 }) }),
  )
  renderWithProviders(<Composer ticketId={7} />)
  await userEvent.type(screen.getByLabelText(/^reply$/i), 'see attached')
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['abc'], 'a.txt', { type: 'text/plain' }))
  expect(screen.getByText(/a\.txt/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /send reply/i })).toBeDisabled()
  await waitFor(() => expect(screen.getByRole('button', { name: /send reply/i })).toBeEnabled())
  await userEvent.click(screen.getByRole('button', { name: /remove a\.txt/i }))
  expect(screen.queryByText(/a\.txt/)).not.toBeInTheDocument()
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['abc'], 'a.txt', { type: 'text/plain' }))
  await waitFor(() => expect(screen.getByRole('button', { name: /send reply/i })).toBeEnabled())
  await userEvent.click(screen.getByRole('button', { name: /send reply/i }))
  await waitFor(() => expect(replyBody).toMatchObject({ file_ids: [42] }))
})

test('failed upload shows the error and does not block other files', async () => {
  server.use(http.post('/api/v1/files', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { file: 'file type application/x-msdownload is not allowed' } } }, { status: 400 })))
  renderWithProviders(<Composer ticketId={7} />)
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['x'], 'evil.exe', { type: 'application/x-msdownload' }))
  expect(await screen.findByText(/not allowed/i)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /send reply/i })).toBeEnabled()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npm test -- src/components/Composer`
Expected: FAIL, modules not found.

- [ ] **Step 3: Implement**

`app/src/components/FileUpload.tsx`:

```tsx
import { useRef, type ChangeEvent } from 'react'
import { ApiError } from '../api/client'
import { uploadFile } from '../api/files'

export interface PendingFile { key: string; name: string; status: 'uploading' | 'done' | 'error'; fileId?: number; error?: string }

export function FileUpload({ pending, onChange, inputId }: { pending: PendingFile[]; onChange: (next: PendingFile[]) => void; inputId: string }) {
  const ref = useRef<PendingFile[]>(pending)
  ref.current = pending

  function update(key: string, patch: Partial<PendingFile>) {
    const next = ref.current.map((p) => (p.key === key ? { ...p, ...patch } : p))
    ref.current = next
    onChange(next)
  }

  async function onPick(e: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files ?? [])
    e.target.value = ''
    for (const file of files) {
      const key = `${file.name}-${Date.now()}-${Math.random()}`
      const next = [...ref.current, { key, name: file.name, status: 'uploading' as const }]
      ref.current = next
      onChange(next)
      try {
        const info = await uploadFile(file)
        update(key, { status: 'done', fileId: info.id })
      } catch (err) {
        const msg = err instanceof ApiError ? (err.fields.file ?? err.message) : 'upload failed'
        update(key, { status: 'error', error: msg })
      }
    }
  }

  return (
    <div>
      <label htmlFor={inputId}>Attach files</label>{' '}
      <input id={inputId} type="file" multiple onChange={(e) => void onPick(e)} />
      {pending.length > 0 && (
        <ul style={{ margin: '8px 0 0', paddingLeft: 18 }}>
          {pending.map((p) => (
            <li key={p.key}>
              {p.name}{' '}
              {p.status === 'uploading' && <span className="muted">uploading…</span>}
              {p.status === 'error' && <span className="field-error">{p.error}</span>}
              <button type="button" aria-label={`Remove ${p.name}`} onClick={() => onChange(ref.current.filter((x) => x.key !== p.key))} style={{ marginLeft: 8 }}>×</button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
```

`app/src/components/Composer.tsx`:

```tsx
import { useMutation } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { ApiError } from '../api/client'
import { note, reply } from '../api/tickets'
import { useInvalidateTicket } from '../hooks/useTicketMutations'
import { useReferenceData } from '../hooks/useReferenceData'
import styles from './Composer.module.css'
import { ErrorBanner } from './ErrorBanner'
import { FileUpload, type PendingFile } from './FileUpload'

type Tab = 'reply' | 'note'

export function Composer({ ticketId }: { ticketId: number }) {
  const [tab, setTab] = useState<Tab>('reply')
  const [body, setBody] = useState('')
  const [title, setTitle] = useState('')
  const [statusId, setStatusId] = useState<number | ''>('')
  const [pending, setPending] = useState<PendingFile[]>([])
  const { statuses } = useReferenceData()
  const invalidate = useInvalidateTicket(ticketId)

  const fileIds = pending.filter((p) => p.status === 'done' && p.fileId !== undefined).map((p) => p.fileId as number)
  const uploading = pending.some((p) => p.status === 'uploading')

  const m = useMutation({
    mutationFn: () => tab === 'reply'
      ? reply(ticketId, { body, format: 'text', ...(statusId ? { status_id: statusId } : {}), file_ids: fileIds })
      : note(ticketId, { ...(title ? { title } : {}), body, format: 'text', file_ids: fileIds }),
    onSuccess: async () => { setBody(''); setTitle(''); setStatusId(''); setPending([]); await invalidate() },
  })
  const fields = m.error instanceof ApiError ? m.error.fields : {}
  const bannerError = m.error instanceof ApiError && Object.keys(m.error.fields).length > 0 ? null : m.error

  function submit(e: FormEvent) { e.preventDefault(); m.mutate() }

  return (
    <section aria-label="Composer" className="panel">
      <div role="tablist" className={styles.tabs}>
        <button role="tab" type="button" aria-selected={tab === 'reply'} className={tab === 'reply' ? styles.active : ''} onClick={() => setTab('reply')}>Reply</button>
        <button role="tab" type="button" aria-selected={tab === 'note'} className={tab === 'note' ? styles.active : ''} onClick={() => setTab('note')}>Internal note</button>
      </div>
      {bannerError && <ErrorBanner error={bannerError} />}
      <form onSubmit={submit}>
        {tab === 'note' && (
          <div className="field">
            <label htmlFor="note-title">Title</label>
            <input id="note-title" value={title} onChange={(e) => setTitle(e.target.value)} />
            {fields.title && <span className="field-error">{fields.title}</span>}
          </div>
        )}
        <div className="field">
          <label htmlFor="composer-body">{tab === 'reply' ? 'Reply' : 'Note'}</label>
          <textarea id="composer-body" rows={6} value={body} onChange={(e) => setBody(e.target.value)} required />
          {fields.body && <span className="field-error">{fields.body}</span>}
        </div>
        <FileUpload pending={pending} onChange={setPending} inputId="composer-files" />
        {fields.file_ids && <span className="field-error">{fields.file_ids}</span>}
        <div className="row" style={{ marginTop: 12 }}>
          {tab === 'reply' && (
            <label>Set status{' '}
              <select aria-label="Set status" value={statusId} onChange={(e) => setStatusId(e.target.value ? Number(e.target.value) : '')}>
                <option value="">Keep current</option>
                {statuses.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </label>
          )}
          <button type="submit" className="primary" disabled={m.isPending || uploading || !body.trim()}>
            {tab === 'reply' ? 'Send reply' : 'Add note'}
          </button>
        </div>
      </form>
    </section>
  )
}
```

`app/src/components/Composer.module.css`:

```css
.tabs { display: flex; gap: 8px; margin-bottom: 12px; }
.tabs button { border-radius: var(--radius) var(--radius) 0 0; }
.active { border-bottom-color: var(--accent); font-weight: 600; }
```

In `TicketDetailPage.tsx`, replace the Composer placeholder with `<Composer ticketId={id} />`.

- [ ] **Step 4: Run tests, lint, build**

Run: `npm test && npm run lint && npm run build`
Expected: PASS (5 new tests). If the `Set status` select is not found by label in the first test, ensure the `aria-label` is present on the select.

- [ ] **Step 5: Commit**

```bash
git add app/src
git commit -m "feat(app): reply and note composer with immediate file upload" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: New ticket page

**Files:**
- Create: `app/src/pages/NewTicketPage.tsx`, `app/src/pages/NewTicketPage.test.tsx`
- Modify: `app/src/App.tsx` (route `/tickets/new` → `NewTicketPage`; remove the placeholder)

**Interfaces:**
- Consumes: `createTicket`, `useReferenceData`, `useAuth`, `ErrorBanner`, `FileUpload`.
- Produces: `NewTicketPage`.

- [ ] **Step 1: Failing tests**

`app/src/pages/NewTicketPage.test.tsx`:

```tsx
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../App'
import { REFRESH_KEY } from '../api/client'
import { ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'

beforeEach(() => localStorage.setItem(REFRESH_KEY, 'refresh-1'))

test('topic pre-fills department and priority; submit navigates to the new ticket', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...ticketFixture, id: 8, number: '000008', subject: 'Refund please' }, { status: 201 }) }))
  server.use(http.get('/api/v1/tickets/8', () => HttpResponse.json({ ...ticketFixture, id: 8, number: '000008', subject: 'Refund please' })))
  renderWithProviders(<App />, { route: '/tickets/new' })
  await userEvent.selectOptions(await screen.findByLabelText(/^topic$/i), '2')
  expect(screen.getByLabelText(/^department$/i)).toHaveValue('2')
  expect(screen.getByLabelText(/^priority$/i)).toHaveValue('3')
  await userEvent.type(screen.getByLabelText(/^subject$/i), 'Refund please')
  await userEvent.type(screen.getByLabelText(/^message$/i), 'I was charged twice')
  await userEvent.type(screen.getByLabelText(/requester name/i), 'Pat')
  await userEvent.type(screen.getByLabelText(/requester email/i), 'pat@example.test')
  await userEvent.selectOptions(screen.getByLabelText(/^source$/i), 'phone')
  await userEvent.click(screen.getByRole('button', { name: /create ticket/i }))
  await waitFor(() => expect(body).toEqual({
    subject: 'Refund please', message: 'I was charged twice', message_format: 'text', requester_name: 'Pat', requester_email: 'pat@example.test',
    topic_id: 2, dept_id: 2, priority_id: 3, source: 'phone', file_ids: [],
  }))
  expect(await screen.findByRole('heading', { name: /000008/ })).toBeInTheDocument()
})

test('validation errors map to fields', async () => {
  server.use(http.post('/api/v1/tickets', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { requester_email: 'email', dept_id: 'required when no topic supplies a department' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/tickets/new' })
  await userEvent.type(await screen.findByLabelText(/^subject$/i), 's')
  await userEvent.type(screen.getByLabelText(/^message$/i), 'm')
  await userEvent.type(screen.getByLabelText(/requester email/i), 'a@b.test')
  await userEvent.click(screen.getByRole('button', { name: /create ticket/i }))
  expect(await screen.findByText(/required when no topic/i)).toBeInTheDocument()
  expect(screen.getByText('email')).toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npm test -- src/pages/NewTicketPage`
Expected: FAIL, module not found.

- [ ] **Step 3: Implement**

`app/src/pages/NewTicketPage.tsx`:

```tsx
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { ApiError } from '../api/client'
import { createTicket } from '../api/tickets'
import type { CreateTicketInput } from '../api/types'
import { ErrorBanner } from '../components/ErrorBanner'
import { FileUpload, type PendingFile } from '../components/FileUpload'
import { useReferenceData } from '../hooks/useReferenceData'

const SOURCES = ['web', 'phone', 'api', 'other'] as const

export function NewTicketPage() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { topics, departments, priorities } = useReferenceData()
  const [form, setForm] = useState({ subject: '', message: '', requester_name: '', requester_email: '', topic_id: 0, dept_id: 0, priority_id: 0, source: 'web', due_at: '' })
  const [pending, setPending] = useState<PendingFile[]>([])
  const uploading = pending.some((p) => p.status === 'uploading')

  const m = useMutation({
    mutationFn: (input: CreateTicketInput) => createTicket(input),
    onSuccess: async (t) => { await qc.invalidateQueries({ queryKey: ['tickets'] }); navigate(`/tickets/${t.id}`) },
  })
  const fields = m.error instanceof ApiError ? m.error.fields : {}
  const bannerError = m.error instanceof ApiError && Object.keys(m.error.fields).length > 0 ? null : m.error

  function onTopic(id: number) {
    const t = topics.find((x) => x.id === id)
    setForm((f) => ({ ...f, topic_id: id, dept_id: t?.dept_id ?? f.dept_id, priority_id: t?.priority_id ?? f.priority_id }))
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    const input: CreateTicketInput = {
      subject: form.subject, message: form.message, message_format: 'text',
      requester_name: form.requester_name, requester_email: form.requester_email,
      ...(form.topic_id ? { topic_id: form.topic_id } : {}),
      ...(form.dept_id ? { dept_id: form.dept_id } : {}),
      ...(form.priority_id ? { priority_id: form.priority_id } : {}),
      source: form.source,
      ...(form.due_at ? { due_at: new Date(form.due_at).toISOString() } : {}),
      file_ids: pending.filter((p) => p.status === 'done' && p.fileId !== undefined).map((p) => p.fileId as number),
    }
    m.mutate(input)
  }

  const field = (id: string, label: string, el: ReactNode, err?: string) => (
    <div className="field"><label htmlFor={id}>{label}</label>{el}{err && <span className="field-error">{err}</span>}</div>
  )

  return (
    <div className="panel" style={{ maxWidth: 640 }}>
      <h1>New ticket</h1>
      {bannerError && <ErrorBanner error={bannerError} />}
      <form onSubmit={submit}>
        {field('subject', 'Subject', <input id="subject" value={form.subject} onChange={(e) => setForm({ ...form, subject: e.target.value })} required maxLength={255} />, fields.subject)}
        {field('message', 'Message', <textarea id="message" rows={6} value={form.message} onChange={(e) => setForm({ ...form, message: e.target.value })} required />, fields.message)}
        {field('rname', 'Requester name', <input id="rname" value={form.requester_name} onChange={(e) => setForm({ ...form, requester_name: e.target.value })} />, fields.requester_name)}
        {field('remail', 'Requester email', <input id="remail" type="email" value={form.requester_email} onChange={(e) => setForm({ ...form, requester_email: e.target.value })} required />, fields.requester_email)}
        {field('topic', 'Topic', <select id="topic" value={form.topic_id} onChange={(e) => onTopic(Number(e.target.value))}>
          <option value={0}>None</option>{topics.filter((t) => t.is_active).map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</select>, fields.topic_id)}
        {field('dept', 'Department', <select id="dept" value={form.dept_id} onChange={(e) => setForm({ ...form, dept_id: Number(e.target.value) })}>
          <option value={0}>Choose…</option>{departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}</select>, fields.dept_id)}
        {field('priority', 'Priority', <select id="priority" value={form.priority_id} onChange={(e) => setForm({ ...form, priority_id: Number(e.target.value) })}>
          <option value={0}>Default</option>{priorities.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select>, fields.priority_id)}
        {field('source', 'Source', <select id="source" value={form.source} onChange={(e) => setForm({ ...form, source: e.target.value })}>
          {SOURCES.map((s) => <option key={s} value={s}>{s}</option>)}</select>, fields.source)}
        {field('due', 'Due', <input id="due" type="datetime-local" value={form.due_at} onChange={(e) => setForm({ ...form, due_at: e.target.value })} />, fields.due_at)}
        <FileUpload pending={pending} onChange={setPending} inputId="new-files" />
        {fields.file_ids && <span className="field-error">{fields.file_ids}</span>}
        <div className="row" style={{ marginTop: 12 }}>
          <button type="submit" className="primary" disabled={m.isPending || uploading}>Create ticket</button>
        </div>
      </form>
    </div>
  )
}
```

Wire `/tickets/new` to `NewTicketPage` in `App.tsx` and delete the placeholder component.

- [ ] **Step 4: Run tests, lint, build**

Run: `npm test && npm run lint && npm run build`
Expected: PASS (2 new tests; roughly 50 in total).

- [ ] **Step 5: Commit**

```bash
git add app/src
git commit -m "feat(app): new ticket page with topic defaults and attachments" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 10: README, run against the real API, final checks

**Files:**
- Create: `app/README.md`
- Modify: `gin/README.md` (one line pointing at `app/`)

**Interfaces:**
- Consumes: everything.
- Produces: a documented, runnable app.

- [ ] **Step 1: README**

`app/README.md`:

```markdown
# Ticket Desk (React frontend)

Agent UI for the Go ticket API in `../gin`. See
`docs/superpowers/specs/2026-09-25-react-agent-frontend-design.md` for the design.

## Run locally

Start the API first (see `../gin/README.md`; it listens on :8080), then:

    npm install
    npm run dev          # http://localhost:5173, proxies /api to :8080

Sign in with an account created by `go run ./cmd/api create-admin` or `POST /staff`.

## Scripts

    npm test             # Vitest + Testing Library + MSW, headless
    npm run lint
    npm run build        # type-check and bundle to dist/

## Layout

`src/api` is the only code that talks to the network (typed client with refresh-on-401).
`src/auth` owns the session. `src/pages` are routes; `src/components` are shared pieces;
`src/hooks` wrap TanStack Query and URL state.
```

Add to `gin/README.md` under Layout: `The React frontend lives in ../app (see its README).`

- [ ] **Step 2: Run against the real API**

With the Go API running on :8080 (`cd ../gin && POSTGRES_PORT=5433 make migrate`, then `DATABASE_URL=... JWT_SECRET=... go run ./cmd/api serve`, per its README) and an admin created:

```bash
npm run dev &
sleep 3
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:5173/          # 200
curl -s http://localhost:5173/api/v1/health                              # {"status":"ok"} via proxy
```

Then, in a browser (or with the Claude in Chrome tools if available): open `http://localhost:5173`, sign in, create a ticket, open it, reply with a status change, upload a file on a note, download it from the thread, filter the list. Record what you saw in the report. Stop the dev server afterwards.

- [ ] **Step 3: Final checks and commit**

Run: `npm test && npm run lint && npm run build`
Expected: all green.

```bash
git add app gin/README.md
git commit -m "docs(app): README and cross-link from the API README" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Done criteria

- `npm test`, `npm run lint`, `npm run build` pass in `app/`.
- Against the real API: sign in, list with filters, open a ticket, reply with status, note with attachment, download, assign, transfer, create ticket all work from the browser.
- Every spec section maps to a task: layout (1), client and auth (2, 3, 4), pages and data flow (5, 6, 7, 8, 9), error and loading states (4 through 9), testing (every task), README (10).
