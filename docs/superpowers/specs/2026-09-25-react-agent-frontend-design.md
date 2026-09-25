# React agent frontend: working tickets

Date: 2026-09-25
Status: approved design, pending implementation plan
Depends on: `docs/superpowers/specs/2026-09-24-go-ticket-api-design.md` (the Go API this app consumes)

## 1. Purpose

A React single-page app for internal support agents, consuming the Go ticket API at
`/api/v1`. This first slice lets an agent log in, work the ticket queue, open a ticket,
read its thread, reply, add internal notes, attach files, change status, assign,
transfer, and create tickets. Admin screens come in a later slice.

### What was agreed

- The app lives in `app/` at the repository root, beside `gin/`.
- Stack: Vite 5, React 18, TypeScript (strict), React Router 6, TanStack Query 5,
  CSS modules with one global stylesheet, no component library.
- Tests: Vitest, Testing Library, MSW mocking the API.
- Development: the Vite dev server on port 5173 proxies `/api` to `http://localhost:8080`,
  so the API needs no CORS entry for local development.
- Scope: working tickets only (option 1 from the brainstorm).

### Assumptions

- Agents authenticate with the API's username and password. The access token is held in
  memory only; the refresh token is stored in `localStorage`.
- There is no design system or brand yet; the UI is plain and functional.
- Replies and notes are composed in a plain textarea and posted as `text` format.
  HTML bodies from the API are rendered read-only after sanitising.

### Success criteria

From a browser an agent can log in, filter and page through tickets, open one, read the
whole thread including attachments, reply (optionally changing status), add a note with an
attachment, assign, transfer, and create a new ticket. Reloading any page keeps the agent
signed in and keeps list filters. `npm test`, `npm run lint`, and `npm run build` pass.

## 2. Project layout and tooling

```
app/
  package.json              vite, react, react-dom, react-router-dom, @tanstack/react-query, dompurify
  vite.config.ts            dev server 5173, proxy /api -> http://localhost:8080
  tsconfig.json             strict, noUncheckedIndexedAccess
  index.html
  src/
    main.tsx                QueryClientProvider, BrowserRouter, AuthProvider
    App.tsx                 routes and the authenticated layout shell (header, nav, outlet)
    api/
      client.ts             request<T>(); bearer header; envelope -> ApiError; refresh-on-401
      types.ts              TypeScript types mirroring the API JSON
      auth.ts               login, refresh, logout, me
      tickets.ts            list, get, create, update, reply, note, thread, setStatus, assign, transfer, events
      reference.ts          priorities, statuses, departments, topics, staff
      files.ts              upload, download (fetch with bearer, save as blob)
    auth/
      AuthContext.tsx       session state and actions; useAuth()
      RequireAuth.tsx       route guard
    pages/
      LoginPage.tsx
      TicketListPage.tsx
      TicketDetailPage.tsx
      NewTicketPage.tsx
    components/
      TicketRow.tsx, StatusBadge.tsx, ThreadEntry.tsx, AttachmentList.tsx,
      FileUpload.tsx, Pagination.tsx, ErrorBanner.tsx, FilterBar.tsx, Composer.tsx,
      TicketHeader.tsx, EventsPanel.tsx
    hooks/
      useReferenceData.ts   priorities, statuses, departments, topics, staff via Query
      useTicketFilters.ts   URL search params <-> ListFilter
    styles/
      global.css            reset, typography, layout tokens
      *.module.css          per page or component
  src/test/
    setup.ts                Testing Library and MSW server lifecycle
    handlers.ts             MSW handlers for every endpoint used
    fixtures.ts             sample tickets, entries, staff
```

Scripts: `npm run dev`, `npm run build` (type-check then Vite build), `npm test`
(Vitest, headless), `npm run lint` (ESLint with the React and TypeScript presets).

Rules:

- `src/api` is the only place that calls `fetch`. Components and pages use the
  functions in `src/api/*.ts` through TanStack Query hooks.
- No `any` in `src/api`. Types in `types.ts` mirror the API field names exactly
  (`snake_case`) and are not renamed at the boundary.
- Each page owns its layout; shared visual pieces live in `components/`.

## 3. API client and authentication

### Request wrapper

`request<T>(method, path, options)` in `client.ts`:

- Prefixes `/api/v1`, sets `Authorization: Bearer <access token>` when one is held,
  sends JSON bodies (or `FormData` for uploads, without a content type header), and
  appends query params from an object, dropping empty values.
- Parses JSON responses into `T`. Non-2xx responses with the envelope
  `{error: {code, message, fields}}` become `ApiError {status, code, message, fields}`.
  Responses without a parsable envelope become `ApiError` with code `network`.
- Timestamps stay ISO strings; formatting happens at render with `Intl.DateTimeFormat`.

### Token handling

- Access token in a module-level variable. Refresh token in `localStorage` under one key.
- On a 401 from any request except `/auth/*`, the client calls `POST /auth/refresh`
  once, stores the new pair, and retries the original request once. Concurrent 401s
  share a single in-flight refresh promise. If the refresh fails, the client clears the
  session and notifies `AuthProvider`, which routes to `/login` with a
  "session expired" notice.
- On app start, if a refresh token exists, the client refreshes to obtain an access
  token and calls `GET /me`. Until that resolves the app renders a loading screen, not
  the login page.
- Logout calls `POST /auth/logout` with the refresh token, then clears both tokens.

### AuthProvider

Exposes `{status: 'loading' | 'anonymous' | 'authenticated', staff, isAdmin,
departmentIds, login(username, password), logout()}`. `RequireAuth` renders its
children when authenticated, redirects to `/login` (remembering the target) when
anonymous, and renders the loading screen while loading.

## 4. Pages and data flow

### Routes

| Route | Page | Guard |
|---|---|---|
| `/login` | LoginPage | none; redirects to `/tickets` if already authenticated |
| `/tickets` | TicketListPage | RequireAuth |
| `/tickets/new` | NewTicketPage | RequireAuth |
| `/tickets/:id` | TicketDetailPage | RequireAuth |
| `/` | redirect to `/tickets` | |
| `*` | NotFound | |

### Login

Username and password fields. Submit calls `login()`. Envelope field errors show under
the matching field; a 401 shows "Invalid username or password". Success redirects to the
remembered target or `/tickets`.

### Ticket list

- Filters are bound to URL search params through `useTicketFilters`, so reload and
  sharing keep them: `state`, `status`, `dept_id`, `assigned_to` (All, Me, Unassigned, or
  a staff id), `q` (debounced 300 ms), `sort`, `page`, `page_size`.
- Columns: number, subject, requester, department, status (badge), priority, assignee,
  last message time. Clicking `created_at`, `last_message_at`, or `priority` headers
  toggles sort and direction. Rows link to the detail page.
- The query key is the full filter object; changing any filter refetches.
  `placeholderData: keepPreviousData` keeps the table visible while paging.
- Header has a "New ticket" button.

### Ticket detail

- **Header** (`TicketHeader`): number, subject, requester name and email, status badge,
  department, topic, priority, assignee, created and closed times. Inline actions:
  - status `<select>` posting to `/status`;
  - assign `<select>` listing active staff whose `department_ids` include the ticket's
    department (client-side filter of `GET /staff`), plus "Unassigned";
  - transfer `<select>` of departments visible to the current agent (admins see all);
  - an "Edit" toggle revealing subject, priority, topic, due date, requester fields
    that `PATCH` on save.
- **Thread**: `GET /thread` entries oldest first, each rendered by `ThreadEntry` with a
  type badge (message, response, note), poster, time, body (sanitised HTML via DOMPurify
  for `html`, preformatted text for `text`), and `AttachmentList`. Attachment links call
  a download helper that fetches `/api/v1/files/:id` with the bearer header and saves
  the blob, because plain anchors cannot carry the header. "Load more" appends the next
  page using `next_after`.
- **Composer**: tabs Reply and Internal note. Textarea, optional "Set status" select on
  Reply, file picker that uploads immediately to `POST /files` and lists pending
  attachments with remove buttons; submit posts with `file_ids`. Note tab has an
  optional title. Disabled while submitting.
- **Events panel**: collapsed by default; lists the audit trail from `GET /events`.

### New ticket

Subject, message, requester name and email, topic, department, priority, source, due
date. Choosing a topic pre-fills department and priority from the topic's defaults;
both remain editable. On 201 the app navigates to the new ticket's detail page.

### Mutations and cache

All writes use `useMutation`. On success they invalidate `['ticket', id]`,
`['thread', id]`, `['events', id]`, and `['tickets']`. Validation errors (`400`) map
`fields` onto the form; other errors render in `ErrorBanner` with the envelope message.

## 5. Error handling and loading states

- Every page has three states: loading (skeleton or spinner), error (banner with a
  retry button that refetches), content.
- 404 on a ticket renders "Ticket not found" with a link back to the list.
- Session expiry mid-page is handled by the client's refresh-and-retry; a failed refresh
  routes to `/login` with the "session expired" notice.
- Network errors and 5xx render the banner; the app never shows a raw stack or JSON.

## 6. Testing

Vitest with Testing Library and MSW. Required coverage:

- **Client**: bearer header attached; envelope parsed into `ApiError`; 401 triggers one
  refresh and one retry; concurrent 401s share one refresh call; failed refresh clears
  storage.
- **Auth flow**: login stores tokens and renders the shell; reload with a stored refresh
  token restores the session; logout clears storage and routes to `/login`.
- **List page**: filter controls update the URL and the request query; pagination
  requests the right page; sort toggles direction.
- **Detail page**: thread renders entries and attachments; reply with status posts the
  right body and invalidates; note posts to `/notes`; assign and transfer post and
  refresh the header; validation errors show on the composer.
- **New ticket**: topic selection pre-fills department and priority; 201 navigates.
- **Composer upload**: file picker calls `/files`, pending list shows the file, submit
  sends `file_ids`.

`npm test` runs headless. `npm run build` must pass type checks. A browser end-to-end
suite is out of scope for this slice.

## 7. Out of scope for this slice

Admin screens (departments, topics, staff), profile editing, rich-text editor, real-time
updates or notifications, dark mode, i18n, a production Docker image or static hosting
configuration, and end-to-end browser tests.
