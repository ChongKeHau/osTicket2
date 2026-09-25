# React admin portal: departments, help topics, staff

Date: 2026-09-25
Status: approved design, pending implementation plan
Depends on:
- `docs/superpowers/specs/2026-09-25-react-agent-frontend-design.md` (the app this slice extends)
- `docs/superpowers/specs/2026-09-24-go-ticket-api-design.md` (the API; its admin endpoints are
  already implemented and are not changed by this slice)

## 1. Purpose

Add admin screens to the React app in `app/` so an administrator can manage departments,
help topics, and staff from the browser, using the admin endpoints the Go API already
exposes. Non-admin agents never see these screens.

### What was agreed

- Same app, same stack, no new dependencies. Screens live under `/admin`.
- Each record is edited on its own page (`/admin/staff/3`), so links reload and can be shared.
  Lists and forms are separate pages; no modals.
- The look matches the existing ticket screens: plain and functional.
- No Go API changes. The UI surfaces the API's validation and conflict messages as they are.

### Assumptions

- `useAuth().isAdmin` is the only signal for showing admin UI. The API enforces the real
  permission with 403; the UI treats 403 like any other error.
- Admin lists are small (tens of rows), so there is no search or pagination on them.
- The existing reference queries (`['ref', 'departments']`, `['ref', 'topics']`,
  `['ref', 'staff']`) are the source of truth for list data. Every admin mutation
  invalidates the matching key, so the ticket screens' dropdowns update immediately.

### Success criteria

From a browser an admin can create, edit, and delete a department and a help topic; create
and edit a staff member, set a staff password, deactivate and reactivate a staff member;
see the API's conflict messages (duplicate name, referenced department or topic, last
active admin) inline; and a non-admin sees no Admin link and gets the not-found page at any
`/admin` URL. `npm test`, `npm run lint`, and `npm run build` pass.

## 2. Routes and guards

New routes are nested inside the existing `RequireAuth` and `Layout` routes:

```
/admin                     -> redirect to /admin/departments
/admin/departments         DepartmentListPage
/admin/departments/new     DepartmentFormPage (create)
/admin/departments/:id     DepartmentFormPage (edit)
/admin/topics              TopicListPage
/admin/topics/new          TopicFormPage (create)
/admin/topics/:id          TopicFormPage (edit)
/admin/staff               StaffListPage
/admin/staff/new           StaffFormPage (create)
/admin/staff/:id           StaffFormPage (edit)
```

- `src/auth/RequireAdmin.tsx`: a route element that renders `<Outlet />` when
  `useAuth().isAdmin` is true and `<NotFoundPage />` otherwise. It sits directly under
  `Layout` so the header stays visible.
- `src/pages/admin/AdminLayout.tsx`: renders a sub-nav with three `NavLink`s (Departments,
  Topics, Staff) and an `<Outlet />`. Wraps all `/admin/*` routes.
- `Layout.tsx` gains an "Admin" `NavLink` to `/admin` in the header nav, rendered only when
  `isAdmin` is true.
- A `:id` that is not a positive integer, or that matches no record in the list, renders
  `NotFoundPage`.

## 3. API layer

`src/api/types.ts` gains the input types, mirroring the Go input structs:

```ts
export interface DepartmentInput { name: string; is_public: boolean; manager_id: number | null }
export interface TopicInput {
  name: string; dept_id: number | null; priority_id: number | null; is_active: boolean; sort_order: number
}
export interface CreateStaffInput {
  username: string; email: string; password: string; first_name: string; last_name: string
  is_admin: boolean; primary_dept_id: number; department_ids: number[]
}
export interface UpdateStaffInput {
  email?: string; first_name?: string; last_name?: string; is_admin?: boolean; is_active?: boolean
  primary_dept_id?: number; department_ids?: number[]
}
```

`src/api/admin.ts` wraps `request` from `src/api/client.ts`:

```ts
createDepartment(input: DepartmentInput): Promise<Department>            // POST   /departments
updateDepartment(id, input: Partial<DepartmentInput>): Promise<Department> // PATCH  /departments/:id
deleteDepartment(id): Promise<void>                                       // DELETE /departments/:id
createTopic(input: TopicInput): Promise<Topic>                            // POST   /topics
updateTopic(id, input: Partial<TopicInput>): Promise<Topic>               // PATCH  /topics/:id
deleteTopic(id): Promise<void>                                            // DELETE /topics/:id
createStaff(input: CreateStaffInput): Promise<Staff>                      // POST   /staff
updateStaff(id, input: UpdateStaffInput): Promise<Staff>                  // PATCH  /staff/:id
setStaffPassword(id, password: string): Promise<void>                     // POST   /staff/:id/password
```

Reads keep using `listDepartments`, `listTopics`, `listStaff` from `src/api/reference.ts`.

## 4. Mutation hooks

`src/hooks/useAdminMutations.ts` exports three hooks, one per entity:

```ts
useDepartmentMutations(): { create, update, remove }
useTopicMutations():      { create, update, remove }
useStaffMutations():      { create, update, setPassword }
```

Each member is a TanStack `useMutation` result. On success every mutation invalidates the
entity's `['ref', <entity>]` key. Department mutations also invalidate `['ref', 'topics']`
and `['ref', 'staff']` (a deleted department can no longer be a topic default or a staff
membership) and `['tickets']` (department names appear in ticket rows). Staff mutations
also invalidate `['tickets']` (assignee names).

## 5. Pages and components

All admin pages live in `src/pages/admin/`. Shared pieces:

- `src/components/AdminTable.tsx`: renders a `<table>` from `columns` (header label plus a
  cell renderer) and `rows`, with an optional `actions` renderer per row. No sorting.
- `src/components/FormField.tsx`: a `<label>` wrapping the field with the label text and an
  optional field error rendered below it. Takes `label`, `error`, `children`.
- `src/components/ConfirmDelete.tsx`: a button labelled "Delete" that, when clicked, swaps
  in "Confirm" and "Cancel" buttons inline. "Confirm" calls `onConfirm`; "Cancel" or a
  failed mutation returns it to the initial state. Disabled while `busy` is true. No browser
  dialogs.

List pages (`DepartmentListPage`, `TopicListPage`, `StaffListPage`):

- Read the reference query; show `LoadingScreen` while loading and `ErrorBanner` with retry
  on error.
- A "New …" link to the create route above the table.
- Each row's name links to its edit page. Departments and topics have a `ConfirmDelete` in
  the actions column. Staff rows show an "Inactive" badge when `is_active` is false, and no
  delete.
- Columns: departments show name, public (Yes/No), manager name; topics show name,
  department, priority, active, sort order; staff show name, username, email, admin, active,
  primary department.
- A delete error (409 or other) shows in an `ErrorBanner` above the table.

Form pages (`DepartmentFormPage`, `TopicFormPage`, `StaffFormPage`):

- One component each, handling both create and edit based on the `:id` param. In edit mode
  the form seeds from the record found in the cached list; if the list is not loaded yet the
  page shows `LoadingScreen`.
- Local `useState` for the form values; submit calls `create` or `update` from the entity's
  mutation hook, then navigates to the list.
- Update sends only fields whose value differs from the loaded record. If nothing changed,
  the page navigates back without a request.
- A "Cancel" link back to the list.
- Field errors from `ApiError.fields` render under the matching `FormField`. Any other error
  renders in an `ErrorBanner` above the form.

Fields:

- Department: name (text, required), public (checkbox), manager (select of staff, with a
  "None" option; sends `manager_id: null` for none).
- Topic: name (text, required), default department (select of departments with "None"),
  default priority (select of priorities with "None"), active (checkbox, default checked),
  sort order (number, default 0).
- Staff, create: username, email, password, first name, last name, admin (checkbox),
  primary department (select, required), departments (checkbox list). Username and password
  are shown only in create mode.
- Staff, edit: email, first name, last name, admin, active, primary department, departments,
  plus a separate "Set password" section with its own password input and button that posts
  through `setPassword` and shows "Password updated" on success. The current admin's own
  row can be edited like any other; the API's last-active-admin rule is surfaced as an error.
- On submit, the primary department is always included in `department_ids` if the admin did
  not tick it.

Styling reuses the global stylesheet's `panel`, `button`, and form rules. One CSS module,
`src/pages/admin/admin.module.css`, holds the sub-nav and table layout.

## 6. Error handling

- 400 `validation_failed`: field map shown under the fields. Unknown field names fall back to
  the banner.
- 409 `conflict`: the API's message in the banner ("department name already exists",
  "department is referenced by tickets, staff or topics", "topic is referenced by tickets",
  "username or email already exists", "cannot remove the last active admin").
- 403: the banner shows the API message. The Admin link remains until the next session
  refresh updates `isAdmin`; no special handling.
- 404 on update or delete (record removed elsewhere): the banner shows the message and the
  list is invalidated so the stale row disappears.
- Buttons are disabled while their mutation is pending.

## 7. Testing

Vitest, Testing Library, MSW, following the existing suite. New MSW handlers cover the nine
admin endpoints with fixture-based responses; tests override per case with `server.use`.
Fixtures add a third department and an inactive staff member.

- `RequireAdmin.test.tsx`: non-admin sees the not-found heading; admin sees the child route.
- `Layout` test: Admin link present for admins, absent for non-admins.
- Each list page: rows render from fixtures; delete confirm flow calls the endpoint and
  invalidates the list; a 409 delete shows the API message; staff list shows the Inactive
  badge and no delete.
- Each form page: create submits the expected JSON body and navigates to the list; edit
  seeds fields and sends only changed fields; a field error renders under the input; a 409
  renders in the banner; an unknown `:id` renders not found.
- Staff form: password field only in create mode; set-password section posts to
  `/staff/:id/password` and shows the success message; the primary department is added to
  `department_ids` when not ticked.
- `useAdminMutations`: department update invalidates departments, topics, staff, and
  tickets keys (assert with a spy on `invalidateQueries`).

`npm test`, `npm run lint`, and `npm run build` must pass. A live headless Chrome pass
against the running API is done at the end of implementation but is not part of the suite.

## 8. Out of scope for this slice

Profile self-editing, audit log views, bulk actions, admin list search or pagination,
priority and status management, and any Go API changes.
