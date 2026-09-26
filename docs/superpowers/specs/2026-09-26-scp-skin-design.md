# Staff panel re-skin and dashboard — design

Date: 2026-09-26. Branch: `scp-skin` from `main` (eb96c562). Precedes the customer portal.

## 1. Purpose

The React agent app (`app/`) works but looks nothing like osTicket. This slice restructures
every existing screen to osTicket's staff control panel (SCP) anatomy — header bar, primary
tabs, sub-nav, content panel, list tables, section-headed form tables, sticky action bars,
tabbed reply composer — with a modern skin rather than a pixel copy, and adds the Dashboard
tab osTicket agents expect. An osTicket agent should recognise every page; nothing should
look like 2012.

Decisions made in brainstorming: re-skin agent and admin before building the customer
portal; same structure as osTicket's SCP, modern styling; existing screens plus Dashboard
(one new API endpoint); design tokens plus CSS modules, no UI library, no chart library.

## 2. Visual foundation

### 2.1 Tokens

`app/src/styles/tokens.css` declares every design value on `:root`. No CSS module may
contain a colour literal (`#…`, `rgb(`, `hsl(`), a font-family, or a pixel radius; a test
greps for them. Values:

| Token | Value | Origin |
|---|---|---|
| `--page-bg` | `#f4f5f7` | osTicket `#eee`, lightened |
| `--panel-bg` | `#ffffff` | |
| `--border` | `#d9dde3` | osTicket `#ddd` |
| `--border-strong` | `#aab2bd` | osTicket `#aaa` |
| `--text` | `#1f2328` | |
| `--text-muted` | `#6b7280` | |
| `--accent` | `#1a5f9a` | osTicket link `#184E81` |
| `--accent-bg` | `#eaf2fa` | active tab background |
| `--header-action` | `#e65524` | osTicket header link |
| `--zebra` | `#f5f9fd` | osTicket `#f0faff` |
| `--row-hover` | `#fffbe6` | osTicket `#FFFFDD` |
| `--note-bg` | `#fff8d6` | internal note background |
| `--green` | `#2e8b57` | osTicket green button |
| `--danger` | `#c0392b` | |
| `--info-bg` / `--info-fg` | `#d9edf7` / `#3a87ad` | osTicket info banner |
| `--notice-bg` / `--notice-fg` | `#dff0d8` / `#2f6f3e` | osTicket notice banner |
| `--warning-bg` / `--warning-fg` | `#fff9d6` / `#8a6d1d` | osTicket warning banner |
| `--error-bg` / `--error-fg` | `#fdecea` / `#a12622` | osTicket error banner |
| `--radius` | `4px` | |
| `--font` | `"Lato", "Helvetica Neue", Arial, sans-serif` | osTicket |
| `--font-size` | `14px` | osTicket |
| `--content-max` | `1100px` | osTicket 960px, widened |
| `--space-1..6` | `4, 8, 12, 16, 24, 32px` | |

Lato is loaded from Google Fonts in `index.html` with `font-display: swap`; the fallback
stack renders when offline. Dark mode is out of scope.

### 2.2 Primitives

All in `app/src/ui/`, one component per file with a CSS module and a Vitest test. Every
existing component that overlaps (Layout, AdminLayout, AdminTable, FormField, Pagination,
StatusBadge, ErrorBanner) is replaced, not kept alongside.

| Component | Props | Behaviour |
|---|---|---|
| `AppShell` | `panel: "agent" \| "admin"`, `children` | Header bar (site name link left; right: agent name, "Admin Panel"/"Agent Panel" switch when `isAdmin`, "Log Out"), `TabBar` for the panel, `SubNav` outlet, centred content panel (`max-width: var(--content-max)`, white, 1px border, page background behind). Renders `Banner` from a `useBanner()` context so any page can flash a message after navigation. |
| `TabBar` | `tabs: {label, to}[]` | Primary tabs; active = bold, accent colour, `--accent-bg` background, 2px accent bottom border. Matched by route prefix. |
| `SubNav` | `items: {label, to, active?}[]`, `right?: ReactNode` | Grey strip under the tabs; active item bold. `right` holds an action such as "New Ticket". |
| `StickyBar` | `title`, `count?`, `actions?` | Title (h2) left, optional "(n)" count, actions right; `position: sticky; top: 0` inside the panel. |
| `ListTable<T>` | `columns: {key, label, sortKey?, width?, render}[]`, `rows`, `sort`, `onSort`, `rowHref?`, `footer?` | Sortable headers show ▲/▼ glyph for the active sort and a muted ▲▼ for sortable inactive ones; zebra rows; hover; whole row is a link when `rowHref`; footer slot for count and `Pagination`. Empty state row "No records found". |
| `FormTable` | `sections: {title, rows: {label, required?, error?, help?, control}[]}[]` | Section heading rows (grey band), 180px label column, required label bold with a red asterisk, error text under the control in `--danger`. |
| `FormActions` | `onReset?`, `cancelTo`, `saving`, `saveLabel="Save Changes"` | Save (primary), Reset, Cancel buttons. |
| `Banner` | `level: info\|notice\|warning\|error`, `children`, `onDismiss?` | Coloured strip with icon glyph; role `alert` for warning/error, `status` otherwise. |
| `Button` | `variant: default\|primary\|add\|danger`, `size?` | `add` is green. Also `LinkButton`. |
| `Menu` | `label`, `items: {label, onSelect, danger?, disabled?}[]` | Dropdown opened by click, closed on Escape/outside click, keyboard navigable (arrow keys, Enter). |
| `Badge` | `color?`, `children` | Small pill; used for priority (uses priority `color`) and status. |
| `Tabs` | `tabs: {id, label}[]`, `active`, `onChange`, `children` | In-page tabs (composer, statistics). |
| `Pagination` | `page`, `pageSize`, `total`, `onPage` | "Showing 1–25 of 132" plus Prev/numbers/Next. |
| `InlineEdit` | `value: ReactNode`, `editor: ReactNode`, `onSave`, `saving` | Click the value to reveal the editor with Save/Cancel; Escape cancels. |

## 3. Navigation

Routes are unchanged except the additions marked new.

Agent panel tabs: **Dashboard** (`/dashboard`, new), **Tickets** (`/tickets`). `/` redirects
to `/tickets`.

Tickets sub-nav (each sets query params on `/tickets`): Open (`state=open`), My Tickets
(`state=open&assigned_to=me`), Unassigned (`state=open&assigned_to=none`), Closed
(`state=closed`), All (no filter). Right slot: "New Ticket" → `/tickets/new`.

Admin panel tabs (admins only, `RequireAdmin` unchanged): **Dashboard** (`/dashboard`, same
page), **Departments** (`/admin/departments`), **Help Topics** (`/admin/topics`), **Staff**
(`/admin/staff`), **Email** (`/admin/email/templates`, new). Sub-nav on each list tab holds
"Add New …" on the right. Email sub-nav: Templates, Outbox, Inbound Log.

> **Amended (as shipped):** admin lists have one "Add New …" control, the green button in the sticky bar; the admin sub-nav right slot is empty.

The header switch links to `/admin/departments` from the agent panel and to `/tickets` from
the admin panel.

## 4. Screens

### 4.1 Login

Centred card on the page background: site name, username, password, Sign In. Error shown as
an inline `Banner level="error"`. Behaviour unchanged.

### 4.2 Ticket queue (`/tickets`)

Above the sticky bar: a search form (`q`) with a Search button. Sticky bar title is the
sub-nav name ("Open Tickets", "My Tickets", "Unassigned Tickets", "Closed Tickets", "All
Tickets") or "Search Results" when `q` is set, with the total count; actions: department
filter select (agent's visible departments) and status select (existing `statuses`
endpoint).

`ListTable` columns, in order: Number (monospace, link), Date (`created_at`, sortable
`created_at`), Subject (bold when `is_answered` is false, as osTicket), From
(`requester_name` or email), Priority (`Badge` with `priority.color`, sortable `priority`),
Department, Assigned To (assignee name or "—"). Last Message (`last_message_at`, sortable
`last_message_at`, default sort `-last_message_at`). Sort keys are exactly those the API
accepts; the header click toggles direction. Row click opens `/tickets/:id`. Footer:
"Showing a–b of n" and `Pagination`. Empty state as in `ListTable`.

> **Amended (as shipped):** the sub-nav active item and the title match on `state` and `assigned_to` only (other params such as `page`, `sort`, `q`, `status`, `dept_id` are ignored).

### 4.3 Ticket view (`/tickets/:id`)

`StickyBar` title "Ticket #<number>"; actions right: **Post Reply**, **Post Note** (both
scroll to the composer and select the matching tab), **Assign** (opens `Menu` of active
staff in the ticket's department plus "Unassign"), **Transfer** (`Menu` of departments),
status `Menu` (statuses, current one disabled), **More** `Menu` with "Edit" (reveals the
inline editors for subject, priority, topic, due date) — every action calls the existing
endpoints (`assign`, `transfer`, `setStatus`, `updateTicket`).

Below: subject as `h3`. Then two side-by-side info tables (stack under 768px):

| Left | Right |
|---|---|
| Status (InlineEdit → status select) | User (`requester_name`) |
| Priority (InlineEdit → priority select) | Email (`mailto:` link) |
| Department (InlineEdit → department select, triggers transfer) | Source |
| Created (absolute + relative) | Assigned To (InlineEdit → staff select) |
| Due Date (InlineEdit → datetime) | Help Topic |
| Closed (when set) | Last Updated |

Thread: entries in a single column; `message` entries have the poster header left-aligned
with a user glyph, `response` entries right-aligned header with the agent's name, `note`
entries on `--note-bg` labelled "Internal Note". Header shows poster, absolute timestamp
with relative time in `title`. Body is the existing sanitised HTML; attachments listed
beneath with size and download link. "Load more" keeps the existing paging. Under the
thread a collapsible **History** section lists ticket events (existing `EventsPanel`
content) and is collapsed by default.

Composer (`id="composer"`): `Tabs` Reply | Internal Note. Reply tab: To (read-only
requester email), Response (existing `Composer` editor), "Set status to" select (optional;
default "— keep —"), attachments (existing `FileUpload`), **Post Reply**. Note tab: Note
editor, attachments, **Post Note**. On success the thread refetches, the composer clears,
and a `notice` banner appears. Posting a reply with a status change calls `reply` then
`setStatus`, in that order; if the second fails the banner says the reply was posted but
the status was not changed.

> **Amended (as shipped):** "More → Edit" is replaced by inline edits on the info tables (subject, help topic, priority, due date, department, assignee, status, requester name and email); a reply with a status change sends `status_id` in the single reply call (no partial-failure state); thread entries show the absolute time with the relative time in the tooltip.

### 4.4 New ticket (`/tickets/new`)

`FormTable` with sections **User Information** (Name, Email required) and **Ticket
Details** (Help Topic, Department, Priority, Subject required, Message required with the
editor, Attachments, Due Date, Source select defaulting to `phone`, since an agent is
entering it). `FormActions` with Save Changes → creates and navigates to the ticket with a
notice banner.

### 4.5 Admin lists

Departments, Help Topics, Staff: `StickyBar` with title and count, an `add` `Button`
("Add New Department" etc.) right, `ListTable` with the current columns made sortable
client-side (these lists are small and unpaged today; sorting is in-memory), row link to
the edit form. Delete moves into a per-row **More** `Menu` with the existing confirm dialog.

> **Amended (as shipped):** admin lists have one "Add New …" control, the green button in the sticky bar; the admin sub-nav right slot is empty.

### 4.6 Admin forms

`FormTable` sections:

- Department: **Settings** (Name required, Public yes/no, Manager select of active
  staff). Only fields the API accepts; osTicket's email and access sections have no API
  counterpart and are omitted.
- Help Topic: **Settings** (Name required, Active yes/no, Sort order); **Routing**
  (Department, Priority).
- Staff: **Account** (Username required, Email required, First name, Last name, Password
  required on create; on edit a separate **Change Password** section posting to the
  existing password endpoint); **Permissions** (Administrator, Active); **Departments**
  (Primary department required, Additional departments multi-select).

`FormActions` on every form; Reset restores the loaded values; Cancel returns to the list.
Validation errors from the API map onto rows by field name as today.

### 4.7 Email admin (new pages, existing endpoints)

- Templates (`/admin/email/templates`): `ListTable` of key, subject, updated; row opens
  `/admin/email/templates/:key`, a `FormTable` with Subject, HTML body, Text body and a
  read-only list of available variables; Save calls `PATCH /email/templates/:key`.
- Outbox (`/admin/email/outbox`): status filter (pending/sent/failed), `ListTable` of id,
  ticket number (link), to, subject, status `Badge`, attempts, next attempt, last error
  (truncated, full in `title`); a **Retry** button on failed rows calls the retry endpoint.
  Paged with the API's limit/offset.
- Inbound Log (`/admin/email/inbound`): `ListTable` of received, from, subject, outcome
  `Badge`, ticket number (link when present), reason. Paged.

> **Amended (as shipped):** outbox and inbound rows link by ticket id (the API returns no ticket number there); the template form reads from the list query (there is no GET-by-key); the Variables row lists the exported fields of `mail.Vars`.

### 4.8 Dashboard (`/dashboard`)

Top: period form — Start date (date input, default today minus period) and Period select
(7, 14, 30, 90 days; default 30) with a Refresh button. Then **Ticket Activity**: an inline
SVG line chart (`app/src/ui/LineChart.tsx`, no dependency) of the daily series with four
lines — Opened, Assigned, Closed, Reopened — a legend, y-axis ticks, and x-axis date labels
every n days so they never overlap; hovering a point shows a native `title` tooltip. Then
**Statistics**: `Tabs` Department | Help Topic | Agent over a `ListTable` with columns
Name, Opened, Assigned, Closed, Reopened, sortable client-side, with a totals row.

> **Amended (as shipped):** the page's default start is today − (period − 1), so the window includes today.

## 5. API: dashboard statistics

New package `gin/internal/dashboard/` (handler, service, tests) mounted on the private
group.

`GET /api/v1/dashboard/stats?start=YYYY-MM-DD&period=30`

- `start` optional; default is today minus `period` days in UTC. `period` ∈ {7, 14, 30,
  90}; default 30; anything else → 422 with the existing validation envelope. The window
  is `[start, start + period days)` in UTC.
- Scope: admins see every department; other agents see only tickets whose `dept_id` is in
  their `DeptIDs`, using the same rule as the ticket list.
- Counts come from `ticket_event` joined to `ticket` for the department: `opened` =
  `created` events, `assigned` = `assigned` events, `closed` = `closed` events,
  `reopened` = `reopened` events, bucketed by `date_trunc('day', created_at AT TIME ZONE
  'UTC')`. Breakdowns group by the ticket's current department, help topic (tickets with
  no topic appear as "— none —"), and the event's `staff_id` for the agent table
  (assigned/closed/reopened counts are attributed to the acting agent; opened counts are
  attributed to the agent who created the ticket, so tickets created by email or the
  public API fall under "— system —").

Response:

```json
{
  "start": "2026-08-27",
  "period": 30,
  "series": [{"date": "2026-08-27", "opened": 3, "assigned": 2, "closed": 1, "reopened": 0}],
  "by_department": [{"id": 1, "name": "Support", "opened": 3, "assigned": 2, "closed": 1, "reopened": 0}],
  "by_topic":      [{"id": null, "name": "— none —", "opened": 1, "assigned": 0, "closed": 0, "reopened": 0}],
  "by_staff":      [{"id": null, "name": "— system —", "opened": 1, "assigned": 0, "closed": 0, "reopened": 0}]
}
```

`series` has exactly `period` entries, zero-filled for days without events. Breakdown
rows are omitted when all four counts are zero. Queries live in `gin/db/queries/dashboard.sql`
(four sqlc queries: series, by department, by topic, by staff), each taking `all_depts`,
`dept_ids`, `from`, `to`. No schema change.

> **Amended (as shipped):** a bad `period` or `start` returns 400 through the shared error envelope (not 422); the default window includes today (default start = today − (period − 1)).

## 6. Error handling

- API errors render as an `error` `Banner` at the top of the panel; validation errors
  additionally map to form rows.
- 401 on any request keeps the existing refresh-then-redirect-to-login behaviour.
- Inline edits that fail restore the previous value and show an error banner.
- The dashboard shows an empty chart and "No activity in this period" when every count is
  zero.

## 7. Testing

- Go: `internal/dashboard` tests on testcontainers Postgres: series zero-fill and window
  edges (an event at `start` counts, at `start + period` does not), department scoping
  for a non-admin, topic and staff null attribution, and the 422 on a bad period. `make
  test` stays above 75%.
- React (Vitest + Testing Library + msw, existing setup): a test per primitive (sorting
  glyphs and callbacks, menu keyboard handling, inline edit cancel/save, banner roles,
  pagination maths); per page: queue renders columns, sub-nav sets the expected query
  params, ticket view actions call the right endpoints, composer reply + status order and
  partial-failure banner, admin forms map validation errors, dashboard renders series and
  switches statistics tabs. A `tokens.test.ts` reads every `*.module.css` and fails on a
  colour literal, font-family, or `border-radius` with a px value.
- End-to-end: the Playwright driver script in the scratchpad is extended to walk login →
  dashboard → queue → ticket view (post a note) → admin department form → email templates,
  taking screenshots for the PR. Run manually against the local API before the PR.

## 8. Execution shape

Lanes for parallel implementation (each on its own worktree and branch off `scp-skin`,
merged back in order, final whole-branch review on the merged result):

- A: Go dashboard endpoint (independent).
- B: tokens + primitives (independent).
- After B: C queue + ticket view + new ticket; D admin lists/forms + email pages; E
  dashboard page (needs A's response shape, which this spec fixes, so E does not wait for A).

## 9. Out of scope

Customer portal, queue mass actions and saved custom queues, Users and Tasks tabs,
Knowledgebase, dark mode, layouts below 768px, a new rich-text editor, profile editing,
SLA/overdue and service-time statistics, CSV export.
