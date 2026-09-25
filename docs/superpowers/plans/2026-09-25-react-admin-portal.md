# React Admin Portal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add admin screens for departments, help topics, and staff to the React app in `app/`, consuming the Go API's existing admin endpoints.

**Architecture:** New routes under `/admin` nested inside the existing `RequireAuth` and `Layout`, guarded by a `RequireAdmin` element that renders the not-found page for non-admins. Reads reuse the cached reference queries (`['ref', …]`); writes go through a new `src/api/admin.ts` and per-entity mutation hooks that invalidate those keys so the ticket screens update immediately. Each entity gets a list page and a form page (create and edit) under `src/pages/admin/`, built from three small shared components.

**Tech Stack:** Vite 5, React 18, TypeScript strict, React Router 6, TanStack Query 5, CSS modules, Vitest, Testing Library, MSW. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-25-react-admin-portal-design.md`

## Global Constraints

- All work is under `app/`; run every `npm` command from `app/`. No Go API changes.
- No new npm dependencies.
- TypeScript strict; `npm run lint` (eslint flat config) and `npm run build` (`tsc -b && vite build`) must pass after every task.
- Tests: Vitest with Testing Library and MSW, following `src/test/{setup,handlers,fixtures,render}.ts(x)`. MSW runs with `onUnhandledRequest: 'error'`, so every endpoint a test touches needs a handler. `afterEach` resets handlers and clears `localStorage`, so per-test setup goes in `beforeEach`.
- Reads use the existing query keys and stale times exactly: `['ref', 'departments']`, `['ref', 'topics']`, `['ref', 'priorities']` with `staleTime: 5 * 60_000`; `['ref', 'staff']` with `staleTime: 60_000`.
- Admin lists have no search or pagination. No browser dialogs (`confirm`, `alert`) anywhere.
- Field errors come from `ApiError.fields` (a `Record<string, string>` keyed by JSON field name); everything else goes to `ErrorBanner`.
- Every commit message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Styling reuses `src/styles/global.css` classes: `panel`, `row`, `muted`, `field`, `field-error`, `button.primary`.

## Review Focus

1. A department delete that the API refuses with 409 must show the API's message and leave the row in place, with the delete button usable again (Task 4 test "409 on delete shows the message and keeps the row").
2. A staff edit that unticks the primary department must still send the primary department in `department_ids` (Task 7 test "primary department is always included in memberships").
3. Editing a record and changing nothing must not send a request (Task 5 test "no changes navigates back without a PATCH").
4. A `:id` that is not a positive integer, such as `/admin/topics/abc`, must render the not-found page rather than crash or show an empty form (Task 6 test "unknown or invalid id renders not found").
5. A validation error on a field the form does not render (an unknown key in `fields`) must still be visible, in the banner (Task 3 test for `splitErrors` and Task 5 test "unknown field error falls back to the banner").

---

### Task 1: API types, admin client, fixtures, and MSW handlers

**Files:**
- Modify: `app/src/api/types.ts` (append after the `Staff` interface)
- Create: `app/src/api/admin.ts`
- Modify: `app/src/test/fixtures.ts` (append)
- Modify: `app/src/test/handlers.ts` (append handlers inside the `handlers` array)
- Create: `app/src/test/admin.ts`
- Test: `app/src/api/admin.test.ts`

**Interfaces:**
- Consumes: `request<T>(method, path, { body })` from `src/api/client.ts` (returns `undefined` on 204, throws `ApiError` on non-2xx).
- Produces:
  - Types `DepartmentInput`, `TopicInput`, `CreateStaffInput`, `UpdateStaffInput` in `src/api/types.ts`.
  - `src/api/admin.ts`: `createDepartment(input: DepartmentInput): Promise<Department>`, `updateDepartment(id: number, input: Partial<DepartmentInput>): Promise<Department>`, `deleteDepartment(id: number): Promise<void>`, `createTopic(input: TopicInput): Promise<Topic>`, `updateTopic(id: number, input: Partial<TopicInput>): Promise<Topic>`, `deleteTopic(id: number): Promise<void>`, `createStaff(input: CreateStaffInput): Promise<Staff>`, `updateStaff(id: number, input: UpdateStaffInput): Promise<Staff>`, `setStaffPassword(id: number, password: string): Promise<void>`.
  - Fixtures: `adminProfileFixture: StaffProfile` (id 3, `is_admin: true`), `adminFixtures.departments: Department[]` (Support, Billing, Sales), `adminFixtures.staff: Staff[]` (the three existing plus an inactive `old`).
  - `src/test/admin.ts`: `signInAsAdmin(): void` (stores a refresh token and overrides `/me` and the three list endpoints with `adminFixtures`), to be called in `beforeEach` of every admin page test.

- [ ] **Step 1: Add the input types**

Append to `app/src/api/types.ts` directly after the `export interface Staff …` line:

```ts
export interface DepartmentInput { name: string; is_public: boolean; manager_id: number | null }
export interface TopicInput { name: string; dept_id: number | null; priority_id: number | null; is_active: boolean; sort_order: number }
export interface CreateStaffInput {
  username: string; email: string; password: string; first_name: string; last_name: string
  is_admin: boolean; primary_dept_id: number; department_ids: number[]
}
export interface UpdateStaffInput {
  email?: string; first_name?: string; last_name?: string; is_admin?: boolean; is_active?: boolean
  primary_dept_id?: number; department_ids?: number[]
}
```

- [ ] **Step 2: Write the failing test**

Create `app/src/api/admin.test.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { referenceFixtures } from '../test/fixtures'
import { server } from '../test/setup'
import { createDepartment, createStaff, createTopic, deleteDepartment, deleteTopic, setStaffPassword, updateDepartment, updateStaff, updateTopic } from './admin'

interface Seen { method: string; path: string; body: unknown }
function capture(): Seen[] {
  const seen: Seen[] = []
  const record = async ({ request }: { request: Request }) => {
    const body = request.method === 'DELETE' ? undefined : await request.json()
    seen.push({ method: request.method, path: new URL(request.url).pathname, body })
  }
  server.use(
    http.post('/api/v1/departments', async (c) => { await record(c); return HttpResponse.json({ ...referenceFixtures.departments[0], id: 9 }, { status: 201 }) }),
    http.patch('/api/v1/departments/:id', async (c) => { await record(c); return HttpResponse.json(referenceFixtures.departments[0]) }),
    http.delete('/api/v1/departments/:id', async (c) => { await record(c); return new HttpResponse(null, { status: 204 }) }),
    http.post('/api/v1/topics', async (c) => { await record(c); return HttpResponse.json({ ...referenceFixtures.topics[0], id: 9 }, { status: 201 }) }),
    http.patch('/api/v1/topics/:id', async (c) => { await record(c); return HttpResponse.json(referenceFixtures.topics[0]) }),
    http.delete('/api/v1/topics/:id', async (c) => { await record(c); return new HttpResponse(null, { status: 204 }) }),
    http.post('/api/v1/staff', async (c) => { await record(c); return HttpResponse.json({ ...referenceFixtures.staff[0], id: 9 }, { status: 201 }) }),
    http.patch('/api/v1/staff/:id', async (c) => { await record(c); return HttpResponse.json(referenceFixtures.staff[0]) }),
    http.post('/api/v1/staff/:id/password', async (c) => { await record(c); return new HttpResponse(null, { status: 204 }) }),
  )
  return seen
}

test('department calls hit the admin endpoints with JSON bodies', async () => {
  const seen = capture()
  const created = await createDepartment({ name: 'Sales', is_public: false, manager_id: 1 })
  expect(created.id).toBe(9)
  await updateDepartment(2, { name: 'Billing & Payments' })
  await expect(deleteDepartment(2)).resolves.toBeUndefined()
  expect(seen).toEqual([
    { method: 'POST', path: '/api/v1/departments', body: { name: 'Sales', is_public: false, manager_id: 1 } },
    { method: 'PATCH', path: '/api/v1/departments/2', body: { name: 'Billing & Payments' } },
    { method: 'DELETE', path: '/api/v1/departments/2', body: undefined },
  ])
})

test('topic calls hit the admin endpoints', async () => {
  const seen = capture()
  await createTopic({ name: 'Outages', dept_id: 1, priority_id: 3, is_active: true, sort_order: 5 })
  await updateTopic(1, { is_active: false })
  await deleteTopic(1)
  expect(seen.map((s) => `${s.method} ${s.path}`)).toEqual(['POST /api/v1/topics', 'PATCH /api/v1/topics/1', 'DELETE /api/v1/topics/1'])
  expect(seen[0].body).toEqual({ name: 'Outages', dept_id: 1, priority_id: 3, is_active: true, sort_order: 5 })
})

test('staff calls hit the admin endpoints; set password posts the password only', async () => {
  const seen = capture()
  await createStaff({ username: 'new', email: 'new@example.test', password: 'secret123', first_name: 'New', last_name: 'Person', is_admin: false, primary_dept_id: 1, department_ids: [1, 2] })
  await updateStaff(2, { is_active: false })
  await expect(setStaffPassword(2, 'another123')).resolves.toBeUndefined()
  expect(seen.map((s) => `${s.method} ${s.path}`)).toEqual(['POST /api/v1/staff', 'PATCH /api/v1/staff/2', 'POST /api/v1/staff/2/password'])
  expect(seen[1].body).toEqual({ is_active: false })
  expect(seen[2].body).toEqual({ password: 'another123' })
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd app && npx vitest run src/api/admin.test.ts`
Expected: FAIL, cannot resolve `./admin`.

- [ ] **Step 4: Write the client**

Create `app/src/api/admin.ts`:

```ts
import { request } from './client'
import type { CreateStaffInput, Department, DepartmentInput, Staff, Topic, TopicInput, UpdateStaffInput } from './types'

export function createDepartment(input: DepartmentInput): Promise<Department> { return request('POST', '/departments', { body: input }) }
export function updateDepartment(id: number, input: Partial<DepartmentInput>): Promise<Department> { return request('PATCH', `/departments/${id}`, { body: input }) }
export function deleteDepartment(id: number): Promise<void> { return request('DELETE', `/departments/${id}`) }

export function createTopic(input: TopicInput): Promise<Topic> { return request('POST', '/topics', { body: input }) }
export function updateTopic(id: number, input: Partial<TopicInput>): Promise<Topic> { return request('PATCH', `/topics/${id}`, { body: input }) }
export function deleteTopic(id: number): Promise<void> { return request('DELETE', `/topics/${id}`) }

export function createStaff(input: CreateStaffInput): Promise<Staff> { return request('POST', '/staff', { body: input }) }
export function updateStaff(id: number, input: UpdateStaffInput): Promise<Staff> { return request('PATCH', `/staff/${id}`, { body: input }) }
export function setStaffPassword(id: number, password: string): Promise<void> { return request('POST', `/staff/${id}/password`, { body: { password } }) }
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd app && npx vitest run src/api/admin.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 6: Add fixtures, default handlers, and the admin sign-in helper**

Append to `app/src/test/fixtures.ts`:

```ts
export const adminProfileFixture: StaffProfile = {
  id: 3, username: 'root', email: 'root@example.test', first_name: 'Root', last_name: 'Admin',
  is_admin: true, department_ids: [1],
}

export const adminFixtures = {
  departments: [
    ...referenceFixtures.departments,
    { id: 3, name: 'Sales', is_public: false, manager_id: 1 },
  ] as Department[],
  staff: [
    ...referenceFixtures.staff,
    { id: 4, username: 'old', email: 'old@example.test', first_name: 'Olive', last_name: 'Old', is_admin: false, is_active: false, primary_dept_id: 2, department_ids: [2] },
  ] as Staff[],
}
```

Append these entries inside the `handlers` array in `app/src/test/handlers.ts` (before the closing `]`):

```ts
  http.post('/api/v1/departments', async ({ request }) => HttpResponse.json({ id: 9, is_public: true, manager_id: null, ...(await request.json() as object) }, { status: 201 })),
  http.patch('/api/v1/departments/:id', async ({ request, params }) => HttpResponse.json({ ...referenceFixtures.departments[0], ...(await request.json() as object), id: Number(params.id) })),
  http.delete('/api/v1/departments/:id', () => new HttpResponse(null, { status: 204 })),
  http.post('/api/v1/topics', async ({ request }) => HttpResponse.json({ id: 9, dept_id: null, priority_id: null, is_active: true, sort_order: 0, ...(await request.json() as object) }, { status: 201 })),
  http.patch('/api/v1/topics/:id', async ({ request, params }) => HttpResponse.json({ ...referenceFixtures.topics[0], ...(await request.json() as object), id: Number(params.id) })),
  http.delete('/api/v1/topics/:id', () => new HttpResponse(null, { status: 204 })),
  http.post('/api/v1/staff', async ({ request }) => {
    const body = await request.json() as Record<string, unknown>
    delete body.password
    return HttpResponse.json({ id: 9, is_active: true, department_ids: [], ...body }, { status: 201 })
  }),
  http.patch('/api/v1/staff/:id', async ({ request, params }) => HttpResponse.json({ ...referenceFixtures.staff[0], ...(await request.json() as object), id: Number(params.id) })),
  http.post('/api/v1/staff/:id/password', () => new HttpResponse(null, { status: 204 })),
```

Create `app/src/test/admin.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { REFRESH_KEY } from '../api/client'
import { adminFixtures, adminProfileFixture, referenceFixtures } from './fixtures'
import { server } from './setup'

/** Call in beforeEach: signs the test in as an admin and serves the larger admin fixtures. */
export function signInAsAdmin(): void {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  server.use(
    http.get('/api/v1/me', () => HttpResponse.json(adminProfileFixture)),
    http.get('/api/v1/departments', () => HttpResponse.json({ items: adminFixtures.departments })),
    http.get('/api/v1/topics', () => HttpResponse.json({ items: referenceFixtures.topics })),
    http.get('/api/v1/staff', () => HttpResponse.json({ items: adminFixtures.staff })),
  )
}
```

- [ ] **Step 7: Run the whole suite, lint, and build**

Run: `cd app && npm test && npm run lint && npm run build`
Expected: all previous tests still pass (the default reference fixtures are unchanged), lint clean, build clean.

- [ ] **Step 8: Commit**

```bash
git add app/src/api/types.ts app/src/api/admin.ts app/src/api/admin.test.ts app/src/test/fixtures.ts app/src/test/handlers.ts app/src/test/admin.ts
git commit -m "feat(app): admin API client, fixtures, and handlers

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: Admin mutation hooks

**Files:**
- Create: `app/src/hooks/useAdminMutations.ts`
- Test: `app/src/hooks/useAdminMutations.test.tsx`

**Interfaces:**
- Consumes: the nine functions from `src/api/admin.ts` (Task 1).
- Produces:
  - `useDepartmentMutations(): { create: UseMutationResult<Department, Error, DepartmentInput>; update: UseMutationResult<Department, Error, { id: number; input: Partial<DepartmentInput> }>; remove: UseMutationResult<void, Error, number> }`
  - `useTopicMutations(): { create, update, remove }` with the same shapes over `Topic`/`TopicInput`.
  - `useStaffMutations(): { create: UseMutationResult<Staff, Error, CreateStaffInput>; update: UseMutationResult<Staff, Error, { id: number; input: UpdateStaffInput }>; setPassword: UseMutationResult<void, Error, { id: number; password: string }> }`
  - Every mutation invalidates on `onSettled` (success or failure), so a 404 on a stale row refreshes the list.

- [ ] **Step 1: Write the failing test**

Create `app/src/hooks/useAdminMutations.test.tsx`:

```tsx
import { QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { makeQueryClient } from '../test/render'
import { server } from '../test/setup'
import { useDepartmentMutations, useStaffMutations, useTopicMutations } from './useAdminMutations'

function setup<T>(hook: () => T) {
  const client = makeQueryClient()
  const spy = vi.spyOn(client, 'invalidateQueries')
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  const { result } = renderHook(hook, { wrapper })
  const keys = () => spy.mock.calls.map((c) => (c[0]?.queryKey ?? []).join('.')).sort()
  return { result, keys }
}

test('department update invalidates departments, topics, staff, and tickets', async () => {
  const { result, keys } = setup(() => useDepartmentMutations())
  await result.current.update.mutateAsync({ id: 1, input: { name: 'Help' } })
  await waitFor(() => expect(keys()).toEqual(['ref.departments', 'ref.staff', 'ref.topics', 'tickets']))
})

test('department delete invalidates even when the API returns 404', async () => {
  server.use(http.delete('/api/v1/departments/:id', () => HttpResponse.json({ error: { code: 'not_found', message: 'department not found' } }, { status: 404 })))
  const { result, keys } = setup(() => useDepartmentMutations())
  await expect(result.current.remove.mutateAsync(9)).rejects.toMatchObject({ status: 404 })
  await waitFor(() => expect(keys()).toContain('ref.departments'))
})

test('topic create invalidates topics only', async () => {
  const { result, keys } = setup(() => useTopicMutations())
  await result.current.create.mutateAsync({ name: 'Outages', dept_id: null, priority_id: null, is_active: true, sort_order: 0 })
  await waitFor(() => expect(keys()).toEqual(['ref.topics']))
})

test('staff update invalidates staff and tickets; set password invalidates nothing', async () => {
  const { result, keys } = setup(() => useStaffMutations())
  await result.current.update.mutateAsync({ id: 1, input: { is_active: false } })
  await waitFor(() => expect(keys()).toEqual(['ref.staff', 'tickets']))
  await result.current.setPassword.mutateAsync({ id: 1, password: 'secret123' })
  expect(keys()).toEqual(['ref.staff', 'tickets'])
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd app && npx vitest run src/hooks/useAdminMutations.test.tsx`
Expected: FAIL, cannot resolve `./useAdminMutations`.

- [ ] **Step 3: Write the hooks**

Create `app/src/hooks/useAdminMutations.ts`:

```ts
import { useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { createDepartment, createStaff, createTopic, deleteDepartment, deleteTopic, setStaffPassword, updateDepartment, updateStaff, updateTopic } from '../api/admin'
import type { CreateStaffInput, DepartmentInput, TopicInput, UpdateStaffInput } from '../api/types'

type Ref = 'departments' | 'topics' | 'staff'

function invalidator(qc: QueryClient, refs: Ref[], tickets: boolean) {
  return async () => {
    await Promise.all([
      ...refs.map((r) => qc.invalidateQueries({ queryKey: ['ref', r] })),
      ...(tickets ? [qc.invalidateQueries({ queryKey: ['tickets'] })] : []),
    ])
  }
}

export function useDepartmentMutations() {
  const qc = useQueryClient()
  // Departments appear in ticket rows, as topic defaults, and as staff memberships.
  const onSettled = invalidator(qc, ['departments', 'topics', 'staff'], true)
  return {
    create: useMutation({ mutationFn: (input: DepartmentInput) => createDepartment(input), onSettled }),
    update: useMutation({ mutationFn: ({ id, input }: { id: number; input: Partial<DepartmentInput> }) => updateDepartment(id, input), onSettled }),
    remove: useMutation({ mutationFn: (id: number) => deleteDepartment(id), onSettled }),
  }
}

export function useTopicMutations() {
  const qc = useQueryClient()
  const onSettled = invalidator(qc, ['topics'], false)
  return {
    create: useMutation({ mutationFn: (input: TopicInput) => createTopic(input), onSettled }),
    update: useMutation({ mutationFn: ({ id, input }: { id: number; input: Partial<TopicInput> }) => updateTopic(id, input), onSettled }),
    remove: useMutation({ mutationFn: (id: number) => deleteTopic(id), onSettled }),
  }
}

export function useStaffMutations() {
  const qc = useQueryClient()
  // Staff names appear as ticket assignees.
  const onSettled = invalidator(qc, ['staff'], true)
  return {
    create: useMutation({ mutationFn: (input: CreateStaffInput) => createStaff(input), onSettled }),
    update: useMutation({ mutationFn: ({ id, input }: { id: number; input: UpdateStaffInput }) => updateStaff(id, input), onSettled }),
    setPassword: useMutation({ mutationFn: ({ id, password }: { id: number; password: string }) => setStaffPassword(id, password) }),
  }
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd app && npx vitest run src/hooks/useAdminMutations.test.tsx`
Expected: PASS (4 tests).

- [ ] **Step 5: Lint and commit**

Run: `cd app && npm run lint`

```bash
git add app/src/hooks/useAdminMutations.ts app/src/hooks/useAdminMutations.test.tsx
git commit -m "feat(app): admin mutation hooks with cache invalidation

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: Shared pieces: FormField, ConfirmDelete, AdminTable, form helpers

**Files:**
- Create: `app/src/components/FormField.tsx`
- Create: `app/src/components/ConfirmDelete.tsx`
- Create: `app/src/components/AdminTable.tsx`
- Create: `app/src/lib/forms.ts`
- Test: `app/src/components/ConfirmDelete.test.tsx`, `app/src/components/AdminTable.test.tsx`, `app/src/lib/forms.test.ts`

**Interfaces:**
- Consumes: `ApiError` from `src/api/client.ts`.
- Produces:
  - `FormField({ label: string; error?: string; children: ReactNode })` renders `<label class="field">` with the label text, the child control, and the error.
  - `CheckboxField({ label: string; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean })` renders an inline checkbox with its label.
  - `ConfirmDelete({ label: string; onConfirm: () => Promise<unknown> })`: a "Delete" button with accessible name `Delete <label>`; clicking swaps in "Confirm" and "Cancel"; Confirm awaits `onConfirm`, disables both while pending, and returns to the initial state afterwards whether it resolved or rejected.
  - `AdminTable<T extends { id: number }>({ columns: Column<T>[]; rows: T[]; actions?: (row: T) => ReactNode; empty?: string })` with `Column<T> = { header: string; cell: (row: T) => ReactNode }`.
  - `lib/forms.ts`: `parseId(raw: string | undefined): number | null` (positive integer or null); `changedFields<T extends object>(before: T, after: T): Partial<T>` (keys of `after` whose value differs from `before`, comparing arrays of numbers order-insensitively); `splitErrors(error: unknown, known: string[]): { fields: Record<string, string>; banner: unknown }` (field errors whose key is in `known` go to `fields`; if `error` is not an `ApiError` with fields, or has any unknown key, `banner` is the error, otherwise `null`).

- [ ] **Step 1: Write the failing helper tests**

Create `app/src/lib/forms.test.ts`:

```ts
import { ApiError } from '../api/client'
import { changedFields, parseId, splitErrors } from './forms'

test('parseId accepts positive integers only', () => {
  expect(parseId('7')).toBe(7)
  expect(parseId('0')).toBeNull()
  expect(parseId('-1')).toBeNull()
  expect(parseId('abc')).toBeNull()
  expect(parseId('1.5')).toBeNull()
  expect(parseId(undefined)).toBeNull()
})

test('changedFields returns only differing keys and ignores array order', () => {
  const before = { name: 'A', is_admin: false, department_ids: [1, 2], primary: 1 }
  const after = { name: 'B', is_admin: false, department_ids: [2, 1], primary: 2 }
  expect(changedFields(before, after)).toEqual({ name: 'B', primary: 2 })
  expect(changedFields(before, { ...before })).toEqual({})
  expect(changedFields(before, { ...before, department_ids: [1] })).toEqual({ department_ids: [1] })
})

test('splitErrors routes known field errors to fields and everything else to the banner', () => {
  const known = ['name', 'is_public']
  const v = new ApiError(400, 'validation_failed', 'request validation failed', { name: 'required' })
  expect(splitErrors(v, known)).toEqual({ fields: { name: 'required' }, banner: null })
  const unknown = new ApiError(400, 'validation_failed', 'request validation failed', { name: 'required', extra: 'bad' })
  expect(splitErrors(unknown, known)).toEqual({ fields: { name: 'required' }, banner: unknown })
  const conflict = new ApiError(409, 'conflict', 'department name already exists')
  expect(splitErrors(conflict, known)).toEqual({ fields: {}, banner: conflict })
  expect(splitErrors(null, known)).toEqual({ fields: {}, banner: null })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd app && npx vitest run src/lib/forms.test.ts`
Expected: FAIL, cannot resolve `./forms`.

- [ ] **Step 3: Write the helpers**

Create `app/src/lib/forms.ts`:

```ts
import { ApiError } from '../api/client'

/** Route params are strings; only a positive integer names a record. */
export function parseId(raw: string | undefined): number | null {
  if (raw === undefined || !/^[1-9]\d*$/.test(raw)) return null
  const n = Number(raw)
  return Number.isSafeInteger(n) ? n : null
}

function normalize(v: unknown): string {
  if (Array.isArray(v)) return JSON.stringify([...v].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0)))
  return JSON.stringify(v ?? null)
}

/** Keys of `after` whose value differs from `before`; number arrays compare as sets. */
export function changedFields<T extends object>(before: T, after: T): Partial<T> {
  const out: Partial<T> = {}
  for (const key of Object.keys(after) as (keyof T)[]) {
    if (normalize(before[key]) !== normalize(after[key])) out[key] = after[key]
  }
  return out
}

/**
 * Field errors for fields the form renders go under the inputs; anything else (a conflict,
 * a network error, or an error on a field the form does not show) goes to the banner.
 */
export function splitErrors(error: unknown, known: string[]): { fields: Record<string, string>; banner: unknown } {
  if (!error) return { fields: {}, banner: null }
  if (!(error instanceof ApiError) || Object.keys(error.fields).length === 0) return { fields: {}, banner: error }
  const fields: Record<string, string> = {}
  let unknown = false
  for (const [k, v] of Object.entries(error.fields)) {
    if (known.includes(k)) fields[k] = v
    else unknown = true
  }
  return { fields, banner: unknown ? error : null }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd app && npx vitest run src/lib/forms.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Write the failing component tests**

Create `app/src/components/ConfirmDelete.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfirmDelete } from './ConfirmDelete'

test('delete asks for confirmation, then calls onConfirm and resets', async () => {
  const onConfirm = vi.fn(() => Promise.resolve())
  render(<ConfirmDelete label="Sales" onConfirm={onConfirm} />)
  await userEvent.click(screen.getByRole('button', { name: 'Delete Sales' }))
  expect(onConfirm).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
  expect(onConfirm).toHaveBeenCalledTimes(1)
  await waitFor(() => expect(screen.getByRole('button', { name: 'Delete Sales' })).toBeInTheDocument())
})

test('cancel returns to the initial state without calling onConfirm', async () => {
  const onConfirm = vi.fn(() => Promise.resolve())
  render(<ConfirmDelete label="Sales" onConfirm={onConfirm} />)
  await userEvent.click(screen.getByRole('button', { name: 'Delete Sales' }))
  await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
  expect(onConfirm).not.toHaveBeenCalled()
  expect(screen.getByRole('button', { name: 'Delete Sales' })).toBeInTheDocument()
})

test('a rejected onConfirm resets to the initial state and disables buttons while pending', async () => {
  let reject!: (e: Error) => void
  const onConfirm = vi.fn(() => new Promise<void>((_, rej) => { reject = rej }))
  render(<ConfirmDelete label="Sales" onConfirm={onConfirm} />)
  await userEvent.click(screen.getByRole('button', { name: 'Delete Sales' }))
  await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
  expect(screen.getByRole('button', { name: /confirm/i })).toBeDisabled()
  expect(screen.getByRole('button', { name: /cancel/i })).toBeDisabled()
  reject(new Error('conflict'))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Delete Sales' })).toBeInTheDocument())
})
```

Create `app/src/components/AdminTable.test.tsx`:

```tsx
import { render, screen, within } from '@testing-library/react'
import { AdminTable } from './AdminTable'

const rows = [{ id: 1, name: 'Support', is_public: true }, { id: 2, name: 'Sales', is_public: false }]
const columns = [
  { header: 'Name', cell: (r: typeof rows[number]) => r.name },
  { header: 'Public', cell: (r: typeof rows[number]) => (r.is_public ? 'Yes' : 'No') },
]

test('renders headers, one row per item, and the actions column', () => {
  render(<AdminTable columns={columns} rows={rows} actions={(r) => <button type="button">Edit {r.name}</button>} />)
  expect(screen.getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Name', 'Public', ''])
  const body = screen.getAllByRole('row').slice(1)
  expect(body).toHaveLength(2)
  expect(within(body[1]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Sales', 'No', 'Edit Sales'])
})

test('renders the empty message when there are no rows', () => {
  render(<AdminTable columns={columns} rows={[]} empty="No departments yet." />)
  expect(screen.getByText('No departments yet.')).toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
})
```

- [ ] **Step 6: Run to verify they fail**

Run: `cd app && npx vitest run src/components/ConfirmDelete.test.tsx src/components/AdminTable.test.tsx`
Expected: FAIL, modules not found.

- [ ] **Step 7: Write the components**

Create `app/src/components/FormField.tsx`:

```tsx
import type { ReactNode } from 'react'

export function FormField({ label, error, children }: { label: string; error?: string; children: ReactNode }) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {error && <span className="field-error">{error}</span>}
    </label>
  )
}

export function CheckboxField({ label, checked, onChange, disabled = false }: { label: string; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <label className="row" style={{ marginBottom: 12 }}>
      <input type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />
      <span>{label}</span>
    </label>
  )
}
```

Create `app/src/components/ConfirmDelete.tsx`:

```tsx
import { useState } from 'react'

/** Inline two-step delete: no browser dialogs. The caller surfaces any error from onConfirm. */
export function ConfirmDelete({ label, onConfirm }: { label: string; onConfirm: () => Promise<unknown> }) {
  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)

  async function confirm() {
    setBusy(true)
    try { await onConfirm() } catch { /* reported by the caller's mutation state */ } finally {
      setBusy(false)
      setConfirming(false)
    }
  }

  if (!confirming) {
    return <button type="button" aria-label={`Delete ${label}`} onClick={() => setConfirming(true)}>Delete</button>
  }
  return (
    <span className="row" style={{ gap: 6 }}>
      <button type="button" disabled={busy} onClick={() => void confirm()} style={{ borderColor: 'var(--danger)', color: 'var(--danger)' }}>Confirm</button>
      <button type="button" disabled={busy} onClick={() => setConfirming(false)}>Cancel</button>
    </span>
  )
}
```

Create `app/src/components/AdminTable.tsx`:

```tsx
import type { ReactNode } from 'react'

export interface Column<T> { header: string; cell: (row: T) => ReactNode }

export function AdminTable<T extends { id: number }>({ columns, rows, actions, empty = 'Nothing here yet.' }: {
  columns: Column<T>[]; rows: T[]; actions?: (row: T) => ReactNode; empty?: string
}) {
  if (rows.length === 0) return <p className="muted">{empty}</p>
  return (
    <table>
      <thead>
        <tr>
          {columns.map((c) => <th key={c.header}>{c.header}</th>)}
          {actions && <th aria-label="Actions" />}
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.id}>
            {columns.map((c) => <td key={c.header}>{c.cell(row)}</td>)}
            {actions && <td style={{ textAlign: 'right' }}>{actions(row)}</td>}
          </tr>
        ))}
      </tbody>
    </table>
  )
}
```

Note on the header test: an empty `<th aria-label="Actions" />` has accessible name "Actions" but empty `textContent`, which is what the test asserts.

- [ ] **Step 8: Run to verify they pass**

Run: `cd app && npx vitest run src/components/ConfirmDelete.test.tsx src/components/AdminTable.test.tsx src/lib/forms.test.ts`
Expected: PASS (8 tests).

- [ ] **Step 9: Lint, build, commit**

Run: `cd app && npm run lint && npm run build`

```bash
git add app/src/components/FormField.tsx app/src/components/ConfirmDelete.tsx app/src/components/ConfirmDelete.test.tsx app/src/components/AdminTable.tsx app/src/components/AdminTable.test.tsx app/src/lib/forms.ts app/src/lib/forms.test.ts
git commit -m "feat(app): shared admin form and table components

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: Admin shell (guard, layout, nav link, routes) and the department list

**Files:**
- Create: `app/src/auth/RequireAdmin.tsx`
- Create: `app/src/pages/admin/AdminLayout.tsx`
- Create: `app/src/pages/admin/admin.module.css`
- Create: `app/src/pages/admin/DepartmentListPage.tsx`
- Modify: `app/src/components/Layout.tsx` (nav)
- Modify: `app/src/App.tsx` (routes)
- Test: `app/src/pages/admin/DepartmentListPage.test.tsx`

**Interfaces:**
- Consumes: `useAuth().isAdmin`; `NotFoundPage`; `useReferenceData()` from `src/hooks/useReferenceData.ts` (returns `{ departments, topics, staff, priorities, statuses, isLoading, error }`); `useDepartmentMutations().remove` (Task 2); `AdminTable`, `ConfirmDelete` (Task 3); `staffName` from `src/lib/format.ts`; `signInAsAdmin` (Task 1).
- Produces: `RequireAdmin` route element; `AdminLayout` with sub-nav; the `/admin` route tree in `App.tsx` that Tasks 5–7 extend; CSS module classes `subnav`, `head`, `form`, `checks` in `admin.module.css`; `DepartmentListPage`.

- [ ] **Step 1: Write the failing test**

Create `app/src/pages/admin/DepartmentListPage.test.tsx`:

```tsx
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { REFRESH_KEY } from '../../api/client'
import { signInAsAdmin } from '../../test/admin'
import { adminFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

test('a non-admin gets not found at /admin and sees no Admin link', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1') // default /me profile is not an admin
  renderWithProviders(<App />, { route: '/admin' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
  expect(screen.queryByRole('link', { name: 'Admin' })).not.toBeInTheDocument()
})

describe('as admin', () => {
  beforeEach(() => signInAsAdmin())

  test('/admin redirects to the department list with the Admin link and sub-nav', async () => {
    renderWithProviders(<App />, { route: '/admin' })
    expect(await screen.findByRole('heading', { name: 'Departments' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Admin' })).toBeInTheDocument()
    const subnav = screen.getByRole('navigation', { name: /admin sections/i })
    expect(within(subnav).getAllByRole('link').map((l) => l.textContent)).toEqual(['Departments', 'Topics', 'Staff'])
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows.map((r) => within(r).getAllByRole('cell')[0].textContent)).toEqual(['Support', 'Billing', 'Sales'])
    expect(within(rows[2]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Sales', 'No', 'Ann Agent', 'Delete'])
    expect(screen.getByRole('link', { name: 'Sales' })).toHaveAttribute('href', '/admin/departments/3')
    expect(screen.getByRole('link', { name: /new department/i })).toHaveAttribute('href', '/admin/departments/new')
  })

  test('confirming a delete calls the endpoint and the row disappears', async () => {
    const deleted: string[] = []
    server.use(http.delete('/api/v1/departments/:id', ({ params }) => {
      deleted.push(String(params.id))
      server.use(http.get('/api/v1/departments', () => HttpResponse.json({ items: adminFixtures.departments.filter((d) => d.id !== 3) })))
      return new HttpResponse(null, { status: 204 })
    }))
    renderWithProviders(<App />, { route: '/admin/departments' })
    await userEvent.click(await screen.findByRole('button', { name: 'Delete Sales' }))
    await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
    await waitFor(() => expect(screen.queryByRole('link', { name: 'Sales' })).not.toBeInTheDocument())
    expect(deleted).toEqual(['3'])
  })

  test('409 on delete shows the message and keeps the row', async () => {
    server.use(http.delete('/api/v1/departments/:id', () =>
      HttpResponse.json({ error: { code: 'conflict', message: 'department is referenced by tickets, staff or topics' } }, { status: 409 })))
    renderWithProviders(<App />, { route: '/admin/departments' })
    await userEvent.click(await screen.findByRole('button', { name: 'Delete Support' }))
    await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
    expect(await screen.findByRole('alert')).toHaveTextContent('department is referenced by tickets, staff or topics')
    expect(screen.getByRole('link', { name: 'Support' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete Support' })).toBeInTheDocument()
  })

  test('a failed department list shows a retryable banner', async () => {
    server.use(http.get('/api/v1/departments', () => HttpResponse.json({ error: { code: 'server_error', message: 'boom' } }, { status: 500 })))
    renderWithProviders(<App />, { route: '/admin/departments' })
    const retry = await screen.findByRole('button', { name: /retry/i })
    server.use(http.get('/api/v1/departments', () => HttpResponse.json({ items: adminFixtures.departments })))
    await userEvent.click(retry)
    expect(await screen.findByRole('link', { name: 'Sales' })).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd app && npx vitest run src/pages/admin/DepartmentListPage.test.tsx`
Expected: FAIL (the non-admin test may pass because `*` already renders not found; the admin tests fail on the missing "Departments" heading).

- [ ] **Step 3: Write the guard, layout, and styles**

Create `app/src/auth/RequireAdmin.tsx`:

```tsx
import { Outlet } from 'react-router-dom'
import { NotFoundPage } from '../pages/NotFoundPage'
import { useAuth } from './AuthContext'

/** Renders the admin routes for admins and the not-found page for everyone else. Sits under RequireAuth. */
export function RequireAdmin() {
  const { isAdmin } = useAuth()
  return isAdmin ? <Outlet /> : <NotFoundPage />
}
```

Create `app/src/pages/admin/admin.module.css`:

```css
.subnav { display: flex; gap: 16px; margin-bottom: 16px; padding-bottom: 8px; border-bottom: 1px solid var(--border); }
.subnav :global(a.active) { font-weight: 600; }
.head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.form { max-width: 560px; }
.checks { display: flex; flex-wrap: wrap; gap: 12px 20px; margin-bottom: 12px; }
.badge { display: inline-block; margin-left: 8px; padding: 1px 6px; border-radius: var(--radius); background: var(--border); font-size: 12px; }
```

Create `app/src/pages/admin/AdminLayout.tsx`:

```tsx
import { NavLink, Outlet } from 'react-router-dom'
import styles from './admin.module.css'

export function AdminLayout() {
  return (
    <div>
      <nav className={styles.subnav} aria-label="Admin sections">
        <NavLink to="/admin/departments">Departments</NavLink>
        <NavLink to="/admin/topics">Topics</NavLink>
        <NavLink to="/admin/staff">Staff</NavLink>
      </nav>
      <Outlet />
    </div>
  )
}
```

In `app/src/components/Layout.tsx`, destructure `isAdmin` from `useAuth()` and add the link after the "New ticket" `NavLink`:

```tsx
  const { staff, isAdmin, logout } = useAuth()
  …
          <NavLink to="/tickets/new">New ticket</NavLink>
          {isAdmin && <NavLink to="/admin">Admin</NavLink>}
```

- [ ] **Step 4: Write the department list page**

Create `app/src/pages/admin/DepartmentListPage.tsx`:

```tsx
import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { AdminTable } from '../../components/AdminTable'
import { ConfirmDelete } from '../../components/ConfirmDelete'
import { ErrorBanner } from '../../components/ErrorBanner'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useDepartmentMutations } from '../../hooks/useAdminMutations'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import styles from './admin.module.css'

export function DepartmentListPage() {
  const qc = useQueryClient()
  const { departments, staff, isLoading, error } = useReferenceData()
  const { remove } = useDepartmentMutations()
  const manager = (id: number | null) => {
    const s = staff.find((x) => x.id === id)
    return s ? staffName(s) : '—'
  }
  return (
    <div>
      <div className={styles.head}>
        <h1 style={{ margin: 0 }}>Departments</h1>
        <Link to="/admin/departments/new"><button type="button" className="primary">New department</button></Link>
      </div>
      {isLoading && <LoadingScreen />}
      {error && <ErrorBanner error={error} onRetry={() => void qc.refetchQueries({ queryKey: ['ref'] })} />}
      {remove.error && <ErrorBanner error={remove.error} />}
      {!isLoading && !error && (
        <AdminTable
          columns={[
            { header: 'Name', cell: (d) => <Link to={`/admin/departments/${d.id}`}>{d.name}</Link> },
            { header: 'Public', cell: (d) => (d.is_public ? 'Yes' : 'No') },
            { header: 'Manager', cell: (d) => manager(d.manager_id) },
          ]}
          rows={departments}
          actions={(d) => <ConfirmDelete label={d.name} onConfirm={() => remove.mutateAsync(d.id)} />}
          empty="No departments yet."
        />
      )}
    </div>
  )
}
```

- [ ] **Step 5: Register the routes**

In `app/src/App.tsx`, add the imports and the admin route tree before the `*` route:

```tsx
import { RequireAdmin } from './auth/RequireAdmin'
import { AdminLayout } from './pages/admin/AdminLayout'
import { DepartmentListPage } from './pages/admin/DepartmentListPage'
…
            <Route path="/tickets/:id" element={<TicketDetailPage />} />
            <Route element={<RequireAdmin />}>
              <Route path="/admin" element={<AdminLayout />}>
                <Route index element={<Navigate to="/admin/departments" replace />} />
                <Route path="departments" element={<DepartmentListPage />} />
              </Route>
            </Route>
            <Route path="*" element={<NotFoundPage />} />
```

- [ ] **Step 6: Run to verify it passes**

Run: `cd app && npx vitest run src/pages/admin/DepartmentListPage.test.tsx`
Expected: PASS (5 tests).

- [ ] **Step 7: Full suite, lint, build, commit**

Run: `cd app && npm test && npm run lint && npm run build`

```bash
git add app/src/auth/RequireAdmin.tsx app/src/pages/admin/AdminLayout.tsx app/src/pages/admin/admin.module.css app/src/pages/admin/DepartmentListPage.tsx app/src/pages/admin/DepartmentListPage.test.tsx app/src/components/Layout.tsx app/src/App.tsx
git commit -m "feat(app): admin shell with guard, sub-nav, and department list

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: Department form page (create and edit)

**Files:**
- Create: `app/src/pages/admin/DepartmentFormPage.tsx`
- Modify: `app/src/App.tsx` (two routes)
- Test: `app/src/pages/admin/DepartmentFormPage.test.tsx`

**Interfaces:**
- Consumes: `useDepartmentMutations().{create, update}` (Task 2); `FormField`, `CheckboxField`, `parseId`, `changedFields`, `splitErrors` (Task 3); `useReferenceData`; `NotFoundPage`; CSS module `form` class (Task 4).
- Produces: `DepartmentFormPage` at `/admin/departments/new` and `/admin/departments/:id`. The pattern (outer page resolves data and record, inner form owns state seeded once) is repeated verbatim for topics and staff in Tasks 6 and 7.

- [ ] **Step 1: Write the failing test**

Create `app/src/pages/admin/DepartmentFormPage.test.tsx`:

```tsx
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { signInAsAdmin } from '../../test/admin'
import { adminFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

beforeEach(() => signInAsAdmin())

test('create submits the form and returns to the list', async () => {
  let body: unknown
  server.use(http.post('/api/v1/departments', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9, ...(body as object) }, { status: 201 }) }))
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  await userEvent.type(await screen.findByLabelText(/^name$/i), 'Sales EMEA')
  await userEvent.click(screen.getByLabelText(/^public$/i))
  await userEvent.selectOptions(screen.getByLabelText(/^manager$/i), '2')
  await userEvent.click(screen.getByRole('button', { name: /^create$/i }))
  await waitFor(() => expect(body).toEqual({ name: 'Sales EMEA', is_public: false, manager_id: 2 }))
  expect(await screen.findByRole('heading', { name: 'Departments' })).toBeInTheDocument()
})

test('edit seeds the form and sends only changed fields', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/departments/3', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...adminFixtures.departments[2], ...(body as object) }) }))
  renderWithProviders(<App />, { route: '/admin/departments/3' })
  expect(await screen.findByRole('heading', { name: 'Edit Sales' })).toBeInTheDocument()
  expect(screen.getByLabelText(/^name$/i)).toHaveValue('Sales')
  expect(screen.getByLabelText(/^public$/i)).not.toBeChecked()
  expect(screen.getByLabelText(/^manager$/i)).toHaveValue('1')
  await userEvent.clear(screen.getByLabelText(/^name$/i))
  await userEvent.type(screen.getByLabelText(/^name$/i), 'Sales EMEA')
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  await waitFor(() => expect(body).toEqual({ name: 'Sales EMEA' }))
  expect(await screen.findByRole('heading', { name: 'Departments' })).toBeInTheDocument()
})

test('no changes navigates back without a PATCH', async () => {
  let calls = 0
  server.use(http.patch('/api/v1/departments/:id', () => { calls++; return HttpResponse.json(adminFixtures.departments[0]) }))
  renderWithProviders(<App />, { route: '/admin/departments/1' })
  await userEvent.click(await screen.findByRole('button', { name: /^save$/i }))
  expect(await screen.findByRole('heading', { name: 'Departments' })).toBeInTheDocument()
  expect(calls).toBe(0)
})

test('clearing the manager sends manager_id null', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/departments/3', async ({ request }) => { body = await request.json(); return HttpResponse.json(adminFixtures.departments[2]) }))
  renderWithProviders(<App />, { route: '/admin/departments/3' })
  await userEvent.selectOptions(await screen.findByLabelText(/^manager$/i), '')
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  await waitFor(() => expect(body).toEqual({ manager_id: null }))
})

test('a known field error renders under the field; an unknown one falls back to the banner', async () => {
  server.use(http.post('/api/v1/departments', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { name: 'required' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  await userEvent.click(await screen.findByRole('button', { name: /^create$/i }))
  expect(await screen.findByText('required')).toBeInTheDocument()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  server.use(http.post('/api/v1/departments', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { something: 'bad' } } }, { status: 400 })))
  await userEvent.click(screen.getByRole('button', { name: /^create$/i }))
  expect(await screen.findByRole('alert')).toHaveTextContent('request validation failed')
})

test('a conflict shows in the banner and keeps the form', async () => {
  server.use(http.post('/api/v1/departments', () =>
    HttpResponse.json({ error: { code: 'conflict', message: 'department name already exists' } }, { status: 409 })))
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  await userEvent.type(await screen.findByLabelText(/^name$/i), 'Support')
  await userEvent.click(screen.getByRole('button', { name: /^create$/i }))
  expect(await screen.findByRole('alert')).toHaveTextContent('department name already exists')
  expect(screen.getByLabelText(/^name$/i)).toHaveValue('Support')
})

test('unknown or invalid id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/departments/999' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})

test('a non-numeric id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/departments/abc' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd app && npx vitest run src/pages/admin/DepartmentFormPage.test.tsx`
Expected: FAIL on the missing form (the two not-found tests may already pass via the `*` route).

- [ ] **Step 3: Write the page**

Create `app/src/pages/admin/DepartmentFormPage.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import type { Department, DepartmentInput, Staff } from '../../api/types'
import { ErrorBanner } from '../../components/ErrorBanner'
import { CheckboxField, FormField } from '../../components/FormField'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useDepartmentMutations } from '../../hooks/useAdminMutations'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import { changedFields, parseId, splitErrors } from '../../lib/forms'
import { NotFoundPage } from '../NotFoundPage'
import styles from './admin.module.css'

const KNOWN = ['name', 'is_public', 'manager_id']
const LIST = '/admin/departments'
const toInput = (d: Department): DepartmentInput => ({ name: d.name, is_public: d.is_public, manager_id: d.manager_id })

export function DepartmentFormPage() {
  const { id } = useParams()
  const { departments, staff, isLoading, error } = useReferenceData()
  if (isLoading) return <LoadingScreen />
  if (error) return <ErrorBanner error={error} />
  let record: Department | undefined
  if (id !== undefined) {
    const n = parseId(id)
    record = n === null ? undefined : departments.find((d) => d.id === n)
    if (!record) return <NotFoundPage />
  }
  // key remounts the form (and reseeds its state) when navigating between records.
  return <DepartmentForm key={record?.id ?? 'new'} record={record} staff={staff} />
}

function DepartmentForm({ record, staff }: { record?: Department; staff: Staff[] }) {
  const navigate = useNavigate()
  const { create, update } = useDepartmentMutations()
  const [form, setForm] = useState<DepartmentInput>(() => (record ? toInput(record) : { name: '', is_public: true, manager_id: null }))
  const set = <K extends keyof DepartmentInput>(k: K, v: DepartmentInput[K]) => setForm((f) => ({ ...f, [k]: v }))
  const active = record ? update : create
  const { fields, banner } = splitErrors(active.error, KNOWN)
  const done = () => navigate(LIST)

  function submit(e: FormEvent) {
    e.preventDefault()
    if (record) {
      const patch = changedFields(toInput(record), form)
      if (Object.keys(patch).length === 0) { done(); return }
      update.mutate({ id: record.id, input: patch }, { onSuccess: done })
    } else {
      create.mutate(form, { onSuccess: done })
    }
  }

  return (
    <form onSubmit={submit} className={`panel ${styles.form}`}>
      <h1 style={{ marginTop: 0 }}>{record ? `Edit ${record.name}` : 'New department'}</h1>
      {banner && <ErrorBanner error={banner} />}
      <FormField label="Name" error={fields.name}>
        <input value={form.name} maxLength={128} onChange={(e) => set('name', e.target.value)} />
      </FormField>
      <CheckboxField label="Public" checked={form.is_public} onChange={(v) => set('is_public', v)} />
      <FormField label="Manager" error={fields.manager_id}>
        <select value={form.manager_id ?? ''} onChange={(e) => set('manager_id', e.target.value ? Number(e.target.value) : null)}>
          <option value="">None</option>
          {staff.map((s) => <option key={s.id} value={s.id}>{staffName(s)}</option>)}
        </select>
      </FormField>
      <div className="row">
        <button type="submit" className="primary" disabled={active.isPending}>{record ? 'Save' : 'Create'}</button>
        <Link to={LIST}>Cancel</Link>
      </div>
    </form>
  )
}
```

In `app/src/App.tsx` import `DepartmentFormPage` and add under the `departments` route:

```tsx
                <Route path="departments/new" element={<DepartmentFormPage />} />
                <Route path="departments/:id" element={<DepartmentFormPage />} />
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd app && npx vitest run src/pages/admin/DepartmentFormPage.test.tsx`
Expected: PASS (8 tests).

- [ ] **Step 5: Full suite, lint, build, commit**

Run: `cd app && npm test && npm run lint && npm run build`

```bash
git add app/src/pages/admin/DepartmentFormPage.tsx app/src/pages/admin/DepartmentFormPage.test.tsx app/src/App.tsx
git commit -m "feat(app): department create and edit form

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: Help topic list and form

**Files:**
- Create: `app/src/pages/admin/TopicListPage.tsx`
- Create: `app/src/pages/admin/TopicFormPage.tsx`
- Modify: `app/src/App.tsx` (three routes)
- Test: `app/src/pages/admin/TopicPages.test.tsx`

**Interfaces:**
- Consumes: `useTopicMutations()` (Task 2); `AdminTable`, `ConfirmDelete`, `FormField`, `CheckboxField`, `parseId`, `changedFields`, `splitErrors` (Task 3); `useReferenceData` (`topics`, `departments`, `priorities`); CSS module classes (Task 4).
- Produces: `TopicListPage` at `/admin/topics`; `TopicFormPage` at `/admin/topics/new` and `/admin/topics/:id`.

- [ ] **Step 1: Write the failing test**

Create `app/src/pages/admin/TopicPages.test.tsx`:

```tsx
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { signInAsAdmin } from '../../test/admin'
import { referenceFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

beforeEach(() => signInAsAdmin())

test('list shows topics with department, priority, active flag, and sort order', async () => {
  renderWithProviders(<App />, { route: '/admin/topics' })
  expect(await screen.findByRole('heading', { name: 'Topics' })).toBeInTheDocument()
  const rows = screen.getAllByRole('row').slice(1)
  expect(rows).toHaveLength(2)
  expect(within(rows[1]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Refunds', 'Billing', 'high', 'Yes', '2', 'Delete'])
  expect(screen.getByRole('link', { name: 'Refunds' })).toHaveAttribute('href', '/admin/topics/2')
  expect(screen.getByRole('link', { name: /new topic/i })).toHaveAttribute('href', '/admin/topics/new')
})

test('delete confirms, calls the endpoint, and a 409 shows the message', async () => {
  const deleted: string[] = []
  server.use(http.delete('/api/v1/topics/:id', ({ params }) => {
    deleted.push(String(params.id))
    server.use(http.get('/api/v1/topics', () => HttpResponse.json({ items: referenceFixtures.topics.filter((t) => t.id !== 2) })))
    return new HttpResponse(null, { status: 204 })
  }))
  renderWithProviders(<App />, { route: '/admin/topics' })
  await userEvent.click(await screen.findByRole('button', { name: 'Delete Refunds' }))
  await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
  await waitFor(() => expect(screen.queryByRole('link', { name: 'Refunds' })).not.toBeInTheDocument())
  expect(deleted).toEqual(['2'])
  server.use(http.delete('/api/v1/topics/:id', () => HttpResponse.json({ error: { code: 'conflict', message: 'topic is referenced by tickets' } }, { status: 409 })))
  await userEvent.click(screen.getByRole('button', { name: 'Delete General Inquiry' }))
  await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
  expect(await screen.findByRole('alert')).toHaveTextContent('topic is referenced by tickets')
  expect(screen.getByRole('link', { name: 'General Inquiry' })).toBeInTheDocument()
})

test('create submits all fields and returns to the list', async () => {
  let body: unknown
  server.use(http.post('/api/v1/topics', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9, ...(body as object) }, { status: 201 }) }))
  renderWithProviders(<App />, { route: '/admin/topics/new' })
  await userEvent.type(await screen.findByLabelText(/^name$/i), 'Outages')
  await userEvent.selectOptions(screen.getByLabelText(/default department/i), '1')
  await userEvent.selectOptions(screen.getByLabelText(/default priority/i), '3')
  await userEvent.click(screen.getByLabelText(/^active$/i))
  await userEvent.clear(screen.getByLabelText(/sort order/i))
  await userEvent.type(screen.getByLabelText(/sort order/i), '5')
  await userEvent.click(screen.getByRole('button', { name: /^create$/i }))
  await waitFor(() => expect(body).toEqual({ name: 'Outages', dept_id: 1, priority_id: 3, is_active: false, sort_order: 5 }))
  expect(await screen.findByRole('heading', { name: 'Topics' })).toBeInTheDocument()
})

test('edit seeds the form and sends only changed fields', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/topics/2', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...referenceFixtures.topics[1], ...(body as object) }) }))
  renderWithProviders(<App />, { route: '/admin/topics/2' })
  expect(await screen.findByRole('heading', { name: 'Edit Refunds' })).toBeInTheDocument()
  expect(screen.getByLabelText(/^name$/i)).toHaveValue('Refunds')
  expect(screen.getByLabelText(/default department/i)).toHaveValue('2')
  expect(screen.getByLabelText(/default priority/i)).toHaveValue('3')
  expect(screen.getByLabelText(/^active$/i)).toBeChecked()
  expect(screen.getByLabelText(/sort order/i)).toHaveValue(2)
  await userEvent.selectOptions(screen.getByLabelText(/default department/i), '')
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  await waitFor(() => expect(body).toEqual({ dept_id: null }))
})

test('field errors render under the fields', async () => {
  server.use(http.post('/api/v1/topics', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { name: 'required', sort_order: 'must be >= 0' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/topics/new' })
  await userEvent.click(await screen.findByRole('button', { name: /^create$/i }))
  expect(await screen.findByText('required')).toBeInTheDocument()
  expect(screen.getByText('must be >= 0')).toBeInTheDocument()
})

test('unknown or invalid id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/topics/abc' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd app && npx vitest run src/pages/admin/TopicPages.test.tsx`
Expected: FAIL on the missing "Topics" heading.

- [ ] **Step 3: Write the list page**

Create `app/src/pages/admin/TopicListPage.tsx`:

```tsx
import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { AdminTable } from '../../components/AdminTable'
import { ConfirmDelete } from '../../components/ConfirmDelete'
import { ErrorBanner } from '../../components/ErrorBanner'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useTopicMutations } from '../../hooks/useAdminMutations'
import { useReferenceData } from '../../hooks/useReferenceData'
import styles from './admin.module.css'

export function TopicListPage() {
  const qc = useQueryClient()
  const { topics, departments, priorities, isLoading, error } = useReferenceData()
  const { remove } = useTopicMutations()
  const name = (list: { id: number; name: string }[], id: number | null) => list.find((x) => x.id === id)?.name ?? '—'
  return (
    <div>
      <div className={styles.head}>
        <h1 style={{ margin: 0 }}>Topics</h1>
        <Link to="/admin/topics/new"><button type="button" className="primary">New topic</button></Link>
      </div>
      {isLoading && <LoadingScreen />}
      {error && <ErrorBanner error={error} onRetry={() => void qc.refetchQueries({ queryKey: ['ref'] })} />}
      {remove.error && <ErrorBanner error={remove.error} />}
      {!isLoading && !error && (
        <AdminTable
          columns={[
            { header: 'Name', cell: (t) => <Link to={`/admin/topics/${t.id}`}>{t.name}</Link> },
            { header: 'Department', cell: (t) => name(departments, t.dept_id) },
            { header: 'Priority', cell: (t) => name(priorities, t.priority_id) },
            { header: 'Active', cell: (t) => (t.is_active ? 'Yes' : 'No') },
            { header: 'Sort', cell: (t) => String(t.sort_order) },
          ]}
          rows={topics}
          actions={(t) => <ConfirmDelete label={t.name} onConfirm={() => remove.mutateAsync(t.id)} />}
          empty="No topics yet."
        />
      )}
    </div>
  )
}
```

- [ ] **Step 4: Write the form page**

Create `app/src/pages/admin/TopicFormPage.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import type { Department, Priority, Topic, TopicInput } from '../../api/types'
import { ErrorBanner } from '../../components/ErrorBanner'
import { CheckboxField, FormField } from '../../components/FormField'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useTopicMutations } from '../../hooks/useAdminMutations'
import { useReferenceData } from '../../hooks/useReferenceData'
import { changedFields, parseId, splitErrors } from '../../lib/forms'
import { NotFoundPage } from '../NotFoundPage'
import styles from './admin.module.css'

const KNOWN = ['name', 'dept_id', 'priority_id', 'is_active', 'sort_order']
const LIST = '/admin/topics'
const toInput = (t: Topic): TopicInput => ({ name: t.name, dept_id: t.dept_id, priority_id: t.priority_id, is_active: t.is_active, sort_order: t.sort_order })

export function TopicFormPage() {
  const { id } = useParams()
  const { topics, departments, priorities, isLoading, error } = useReferenceData()
  if (isLoading) return <LoadingScreen />
  if (error) return <ErrorBanner error={error} />
  let record: Topic | undefined
  if (id !== undefined) {
    const n = parseId(id)
    record = n === null ? undefined : topics.find((t) => t.id === n)
    if (!record) return <NotFoundPage />
  }
  return <TopicForm key={record?.id ?? 'new'} record={record} departments={departments} priorities={priorities} />
}

function TopicForm({ record, departments, priorities }: { record?: Topic; departments: Department[]; priorities: Priority[] }) {
  const navigate = useNavigate()
  const { create, update } = useTopicMutations()
  const [form, setForm] = useState<TopicInput>(() => (record ? toInput(record) : { name: '', dept_id: null, priority_id: null, is_active: true, sort_order: 0 }))
  const set = <K extends keyof TopicInput>(k: K, v: TopicInput[K]) => setForm((f) => ({ ...f, [k]: v }))
  const active = record ? update : create
  const { fields, banner } = splitErrors(active.error, KNOWN)
  const done = () => navigate(LIST)
  const optional = (v: string) => (v ? Number(v) : null)

  function submit(e: FormEvent) {
    e.preventDefault()
    if (record) {
      const patch = changedFields(toInput(record), form)
      if (Object.keys(patch).length === 0) { done(); return }
      update.mutate({ id: record.id, input: patch }, { onSuccess: done })
    } else {
      create.mutate(form, { onSuccess: done })
    }
  }

  return (
    <form onSubmit={submit} className={`panel ${styles.form}`}>
      <h1 style={{ marginTop: 0 }}>{record ? `Edit ${record.name}` : 'New topic'}</h1>
      {banner && <ErrorBanner error={banner} />}
      <FormField label="Name" error={fields.name}>
        <input value={form.name} maxLength={128} onChange={(e) => set('name', e.target.value)} />
      </FormField>
      <FormField label="Default department" error={fields.dept_id}>
        <select value={form.dept_id ?? ''} onChange={(e) => set('dept_id', optional(e.target.value))}>
          <option value="">None</option>
          {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
        </select>
      </FormField>
      <FormField label="Default priority" error={fields.priority_id}>
        <select value={form.priority_id ?? ''} onChange={(e) => set('priority_id', optional(e.target.value))}>
          <option value="">None</option>
          {priorities.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
      </FormField>
      <CheckboxField label="Active" checked={form.is_active} onChange={(v) => set('is_active', v)} />
      <FormField label="Sort order" error={fields.sort_order}>
        <input type="number" value={form.sort_order} onChange={(e) => set('sort_order', Number(e.target.value) || 0)} />
      </FormField>
      <div className="row">
        <button type="submit" className="primary" disabled={active.isPending}>{record ? 'Save' : 'Create'}</button>
        <Link to={LIST}>Cancel</Link>
      </div>
    </form>
  )
}
```

In `app/src/App.tsx` import both pages and add under the department routes:

```tsx
                <Route path="topics" element={<TopicListPage />} />
                <Route path="topics/new" element={<TopicFormPage />} />
                <Route path="topics/:id" element={<TopicFormPage />} />
```

- [ ] **Step 5: Run to verify it passes**

Run: `cd app && npx vitest run src/pages/admin/TopicPages.test.tsx`
Expected: PASS (6 tests). If the sort-order test fails because `userEvent.clear` on a number input leaves `0` and typing yields `05`, change the handler to `set('sort_order', e.target.value === '' ? 0 : Number(e.target.value))` and keep the assertion.

- [ ] **Step 6: Full suite, lint, build, commit**

Run: `cd app && npm test && npm run lint && npm run build`

```bash
git add app/src/pages/admin/TopicListPage.tsx app/src/pages/admin/TopicFormPage.tsx app/src/pages/admin/TopicPages.test.tsx app/src/App.tsx
git commit -m "feat(app): help topic list and form

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: Staff list and form with set-password

**Files:**
- Create: `app/src/pages/admin/StaffListPage.tsx`
- Create: `app/src/pages/admin/StaffFormPage.tsx`
- Modify: `app/src/App.tsx` (three routes)
- Test: `app/src/pages/admin/StaffPages.test.tsx`

**Interfaces:**
- Consumes: `useStaffMutations()` (Task 2); `AdminTable`, `FormField`, `CheckboxField`, `parseId`, `changedFields`, `splitErrors` (Task 3); `useReferenceData` (`staff`, `departments`); `staffName`; CSS module classes `checks`, `badge` (Task 4).
- Produces: `StaffListPage` at `/admin/staff`; `StaffFormPage` at `/admin/staff/new` and `/admin/staff/:id`.

- [ ] **Step 1: Write the failing test**

Create `app/src/pages/admin/StaffPages.test.tsx`:

```tsx
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { signInAsAdmin } from '../../test/admin'
import { adminFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

beforeEach(() => signInAsAdmin())

test('list shows staff with an Inactive badge and no delete buttons', async () => {
  renderWithProviders(<App />, { route: '/admin/staff' })
  expect(await screen.findByRole('heading', { name: 'Staff' })).toBeInTheDocument()
  const rows = screen.getAllByRole('row').slice(1)
  expect(rows).toHaveLength(4)
  expect(within(rows[2]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Root Admin', 'root', 'root@example.test', 'Yes', 'Yes', 'Support'])
  expect(within(rows[3]).getByText('Inactive')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /^delete/i })).not.toBeInTheDocument()
  expect(screen.getByRole('link', { name: /Olive Old/ })).toHaveAttribute('href', '/admin/staff/4')
})

test('create shows username and password, adds the primary department to memberships', async () => {
  let body: unknown
  server.use(http.post('/api/v1/staff', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9, is_active: true, ...(body as object) }, { status: 201 }) }))
  renderWithProviders(<App />, { route: '/admin/staff/new' })
  await userEvent.type(await screen.findByLabelText(/^username$/i), 'newbie')
  await userEvent.type(screen.getByLabelText(/^email$/i), 'newbie@example.test')
  await userEvent.type(screen.getByLabelText(/^password$/i), 'secret123')
  await userEvent.type(screen.getByLabelText(/first name/i), 'New')
  await userEvent.type(screen.getByLabelText(/last name/i), 'Person')
  await userEvent.click(screen.getByLabelText(/^administrator$/i))
  await userEvent.selectOptions(screen.getByLabelText(/primary department/i), '2')
  await userEvent.click(screen.getByLabelText('Sales')) // membership checkbox; Billing (primary) is not ticked
  expect(screen.queryByLabelText(/^active$/i)).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: /^create$/i }))
  await waitFor(() => expect(body).toEqual({
    username: 'newbie', email: 'newbie@example.test', password: 'secret123', first_name: 'New', last_name: 'Person',
    is_admin: true, primary_dept_id: 2, department_ids: [3, 2],
  }))
  expect(await screen.findByRole('heading', { name: 'Staff' })).toBeInTheDocument()
})

test('edit hides username and password, seeds fields, and sends only changes', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/staff/2', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...adminFixtures.staff[1], ...(body as object) }) }))
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  expect(await screen.findByRole('heading', { name: 'Edit Bob Billing' })).toBeInTheDocument()
  expect(screen.queryByLabelText(/^username$/i)).not.toBeInTheDocument()
  expect(screen.queryByLabelText(/^password$/i)).not.toBeInTheDocument()
  expect(screen.getByLabelText(/^email$/i)).toHaveValue('bob@example.test')
  expect(screen.getByLabelText(/primary department/i)).toHaveValue('2')
  expect(screen.getByLabelText('Billing')).toBeChecked()
  expect(screen.getByLabelText(/^active$/i)).toBeChecked()
  await userEvent.click(screen.getByLabelText(/^active$/i))
  await userEvent.click(screen.getByLabelText('Support'))
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  await waitFor(() => expect(body).toEqual({ is_active: false, department_ids: [2, 1] }))
})

test('primary department is always included in memberships', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/staff/2', async ({ request }) => { body = await request.json(); return HttpResponse.json(adminFixtures.staff[1]) }))
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  await userEvent.click(await screen.findByLabelText('Billing')) // untick the primary department
  await userEvent.selectOptions(screen.getByLabelText(/primary department/i), '1')
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  await waitFor(() => expect(body).toEqual({ primary_dept_id: 1, department_ids: [1] }))
})

test('set password posts to the password endpoint and confirms', async () => {
  let body: unknown
  server.use(http.post('/api/v1/staff/2/password', async ({ request }) => { body = await request.json(); return new HttpResponse(null, { status: 204 }) }))
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  await userEvent.type(await screen.findByLabelText(/new password/i), 'another123')
  await userEvent.click(screen.getByRole('button', { name: /set password/i }))
  expect(await screen.findByText('Password updated')).toBeInTheDocument()
  expect(body).toEqual({ password: 'another123' })
  expect(screen.getByLabelText(/new password/i)).toHaveValue('')
})

test('the last-admin conflict shows in the banner', async () => {
  server.use(http.patch('/api/v1/staff/3', () => HttpResponse.json({ error: { code: 'conflict', message: 'cannot remove the last active admin' } }, { status: 409 })))
  renderWithProviders(<App />, { route: '/admin/staff/3' })
  await userEvent.click(await screen.findByLabelText(/^administrator$/i))
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  expect(await screen.findByRole('alert')).toHaveTextContent('cannot remove the last active admin')
})

test('create validation errors render under fields', async () => {
  server.use(http.post('/api/v1/staff', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { email: 'email', primary_dept_id: 'required' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/staff/new' })
  await userEvent.click(await screen.findByRole('button', { name: /^create$/i }))
  expect(await screen.findByText('email')).toBeInTheDocument()
  expect(screen.getByText('required')).toBeInTheDocument()
})

test('unknown id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/staff/999' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd app && npx vitest run src/pages/admin/StaffPages.test.tsx`
Expected: FAIL on the missing "Staff" heading.

- [ ] **Step 3: Write the list page**

Create `app/src/pages/admin/StaffListPage.tsx`:

```tsx
import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { AdminTable } from '../../components/AdminTable'
import { ErrorBanner } from '../../components/ErrorBanner'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import styles from './admin.module.css'

export function StaffListPage() {
  const qc = useQueryClient()
  const { staff, departments, isLoading, error } = useReferenceData()
  const dept = (id: number) => departments.find((d) => d.id === id)?.name ?? '—'
  return (
    <div>
      <div className={styles.head}>
        <h1 style={{ margin: 0 }}>Staff</h1>
        <Link to="/admin/staff/new"><button type="button" className="primary">New staff member</button></Link>
      </div>
      {isLoading && <LoadingScreen />}
      {error && <ErrorBanner error={error} onRetry={() => void qc.refetchQueries({ queryKey: ['ref'] })} />}
      {!isLoading && !error && (
        <AdminTable
          columns={[
            { header: 'Name', cell: (s) => <><Link to={`/admin/staff/${s.id}`}>{staffName(s)}</Link>{!s.is_active && <span className={styles.badge}>Inactive</span>}</> },
            { header: 'Username', cell: (s) => s.username },
            { header: 'Email', cell: (s) => s.email },
            { header: 'Admin', cell: (s) => (s.is_admin ? 'Yes' : 'No') },
            { header: 'Active', cell: (s) => (s.is_active ? 'Yes' : 'No') },
            { header: 'Primary department', cell: (s) => dept(s.primary_dept_id) },
          ]}
          rows={staff}
          empty="No staff yet."
        />
      )}
    </div>
  )
}
```

Note: the name cell's `textContent` for an active row is just the name, which is what the list test asserts for Root Admin.

- [ ] **Step 4: Write the form page**

Create `app/src/pages/admin/StaffFormPage.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import type { Department, Staff, UpdateStaffInput } from '../../api/types'
import { ErrorBanner } from '../../components/ErrorBanner'
import { CheckboxField, FormField } from '../../components/FormField'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useStaffMutations } from '../../hooks/useAdminMutations'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import { changedFields, parseId, splitErrors } from '../../lib/forms'
import { NotFoundPage } from '../NotFoundPage'
import styles from './admin.module.css'

const LIST = '/admin/staff'
const EDIT_KNOWN = ['email', 'first_name', 'last_name', 'is_admin', 'is_active', 'primary_dept_id', 'department_ids']
const CREATE_KNOWN = ['username', 'password', ...EDIT_KNOWN]

/** Editable values; primary_dept_id 0 means "not chosen yet" and is sent as 0 so the API reports it. */
interface Form {
  username: string; email: string; password: string; first_name: string; last_name: string
  is_admin: boolean; is_active: boolean; primary_dept_id: number; department_ids: number[]
}

const fromRecord = (s: Staff): Form => ({
  username: s.username, email: s.email, password: '', first_name: s.first_name, last_name: s.last_name,
  is_admin: s.is_admin, is_active: s.is_active, primary_dept_id: s.primary_dept_id, department_ids: s.department_ids,
})
const EMPTY: Form = { username: '', email: '', password: '', first_name: '', last_name: '', is_admin: false, is_active: true, primary_dept_id: 0, department_ids: [] }

/** The primary department is always a membership. */
const withPrimary = (ids: number[], primary: number) => (primary && !ids.includes(primary) ? [...ids, primary] : ids)

const toUpdate = (f: Form): Required<UpdateStaffInput> => ({
  email: f.email, first_name: f.first_name, last_name: f.last_name, is_admin: f.is_admin, is_active: f.is_active,
  primary_dept_id: f.primary_dept_id, department_ids: withPrimary(f.department_ids, f.primary_dept_id),
})

export function StaffFormPage() {
  const { id } = useParams()
  const { staff, departments, isLoading, error } = useReferenceData()
  if (isLoading) return <LoadingScreen />
  if (error) return <ErrorBanner error={error} />
  let record: Staff | undefined
  if (id !== undefined) {
    const n = parseId(id)
    record = n === null ? undefined : staff.find((s) => s.id === n)
    if (!record) return <NotFoundPage />
  }
  return <StaffForm key={record?.id ?? 'new'} record={record} departments={departments} />
}

function StaffForm({ record, departments }: { record?: Staff; departments: Department[] }) {
  const navigate = useNavigate()
  const { create, update } = useStaffMutations()
  const [form, setForm] = useState<Form>(() => (record ? fromRecord(record) : EMPTY))
  const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm((f) => ({ ...f, [k]: v }))
  const toggleDept = (id: number, on: boolean) => set('department_ids', on ? [...form.department_ids, id] : form.department_ids.filter((d) => d !== id))
  const active = record ? update : create
  const { fields, banner } = splitErrors(active.error, record ? EDIT_KNOWN : CREATE_KNOWN)
  const done = () => navigate(LIST)

  function submit(e: FormEvent) {
    e.preventDefault()
    if (record) {
      const patch = changedFields(toUpdate(fromRecord(record)), toUpdate(form))
      if (Object.keys(patch).length === 0) { done(); return }
      update.mutate({ id: record.id, input: patch }, { onSuccess: done })
    } else {
      create.mutate({
        username: form.username, email: form.email, password: form.password, first_name: form.first_name, last_name: form.last_name,
        is_admin: form.is_admin, primary_dept_id: form.primary_dept_id, department_ids: withPrimary(form.department_ids, form.primary_dept_id),
      }, { onSuccess: done })
    }
  }

  return (
    <>
      <form onSubmit={submit} className={`panel ${styles.form}`}>
        <h1 style={{ marginTop: 0 }}>{record ? `Edit ${staffName(record)}` : 'New staff member'}</h1>
        {banner && <ErrorBanner error={banner} />}
        {!record && (
          <FormField label="Username" error={fields.username}>
            <input value={form.username} maxLength={64} autoComplete="off" onChange={(e) => set('username', e.target.value)} />
          </FormField>
        )}
        <FormField label="Email" error={fields.email}>
          <input type="email" value={form.email} maxLength={255} onChange={(e) => set('email', e.target.value)} />
        </FormField>
        {!record && (
          <FormField label="Password" error={fields.password}>
            <input type="password" value={form.password} autoComplete="new-password" onChange={(e) => set('password', e.target.value)} />
          </FormField>
        )}
        <FormField label="First name" error={fields.first_name}>
          <input value={form.first_name} maxLength={64} onChange={(e) => set('first_name', e.target.value)} />
        </FormField>
        <FormField label="Last name" error={fields.last_name}>
          <input value={form.last_name} maxLength={64} onChange={(e) => set('last_name', e.target.value)} />
        </FormField>
        <CheckboxField label="Administrator" checked={form.is_admin} onChange={(v) => set('is_admin', v)} />
        {record && <CheckboxField label="Active" checked={form.is_active} onChange={(v) => set('is_active', v)} />}
        <FormField label="Primary department" error={fields.primary_dept_id}>
          <select value={form.primary_dept_id || ''} onChange={(e) => set('primary_dept_id', Number(e.target.value) || 0)}>
            <option value="">Choose…</option>
            {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </select>
        </FormField>
        <fieldset style={{ border: 'none', padding: 0, margin: 0 }}>
          <legend className="muted" style={{ marginBottom: 6 }}>Departments</legend>
          <div className={styles.checks}>
            {departments.map((d) => (
              <label key={d.id} className="row" style={{ gap: 6 }}>
                <input type="checkbox" checked={form.department_ids.includes(d.id)} onChange={(e) => toggleDept(d.id, e.target.checked)} />
                <span>{d.name}</span>
              </label>
            ))}
          </div>
          {fields.department_ids && <span className="field-error">{fields.department_ids}</span>}
        </fieldset>
        <div className="row">
          <button type="submit" className="primary" disabled={active.isPending}>{record ? 'Save' : 'Create'}</button>
          <Link to={LIST}>Cancel</Link>
        </div>
      </form>
      {record && <SetPassword id={record.id} />}
    </>
  )
}

function SetPassword({ id }: { id: number }) {
  const { setPassword } = useStaffMutations()
  const [password, setValue] = useState('')
  const [updated, setUpdated] = useState(false)
  const { fields, banner } = splitErrors(setPassword.error, ['password'])

  function submit(e: FormEvent) {
    e.preventDefault()
    setUpdated(false)
    setPassword.mutate({ id, password }, { onSuccess: () => { setValue(''); setUpdated(true) } })
  }

  return (
    <form onSubmit={submit} className={`panel ${styles.form}`}>
      <h2 style={{ marginTop: 0 }}>Set password</h2>
      {banner && <ErrorBanner error={banner} />}
      {updated && <p role="status">Password updated</p>}
      <FormField label="New password" error={fields.password}>
        <input type="password" value={password} autoComplete="new-password" onChange={(e) => setValue(e.target.value)} />
      </FormField>
      <button type="submit" disabled={setPassword.isPending || password === ''}>Set password</button>
    </form>
  )
}
```

In `app/src/App.tsx` import both pages and add under the topic routes:

```tsx
                <Route path="staff" element={<StaffListPage />} />
                <Route path="staff/new" element={<StaffFormPage />} />
                <Route path="staff/:id" element={<StaffFormPage />} />
```

- [ ] **Step 5: Run to verify it passes**

Run: `cd app && npx vitest run src/pages/admin/StaffPages.test.tsx`
Expected: PASS (8 tests). Note for the "sends only changes" test: the record's memberships are `[2]`; ticking Support gives `[2, 1]`, and `changedFields` compares number arrays as sets, so the order in the assertion is the insertion order the form produces.

- [ ] **Step 6: Full suite, lint, build, commit**

Run: `cd app && npm test && npm run lint && npm run build`

```bash
git add app/src/pages/admin/StaffListPage.tsx app/src/pages/admin/StaffFormPage.tsx app/src/pages/admin/StaffPages.test.tsx app/src/App.tsx
git commit -m "feat(app): staff list, form, and set-password

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: README and final verification

**Files:**
- Modify: `app/README.md` (add an "Admin" section after "Run locally")

**Interfaces:**
- Consumes: everything above.
- Produces: documentation and a green tree.

- [ ] **Step 1: Document the admin screens**

Add to `app/README.md` after the "Run locally" section:

```markdown
## Admin

Staff with `is_admin` see an **Admin** link in the header. It opens `/admin` with three
sections: Departments, Topics, and Staff. Each has a list page and a per-record form
(`/admin/<section>/new`, `/admin/<section>/<id>`). Departments and topics can be deleted from
the list (inline confirm; the API refuses deletion while tickets, staff, or topics still
reference them). Staff cannot be deleted; deactivate them instead. The staff edit page also
has a "Set password" section. The API enforces admin rights and the last-active-admin rule;
the UI shows those errors inline.
```

- [ ] **Step 2: Run the full gate**

Run: `cd app && npm test && npm run lint && npm run build`
Expected: all tests pass (76 existing plus the new ones), lint clean, build clean.

- [ ] **Step 3: Commit**

```bash
git add app/README.md
git commit -m "docs(app): describe the admin screens

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```
