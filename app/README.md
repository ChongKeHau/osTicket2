# Ticket Desk (React frontend)

Agent UI for the Go ticket API in `../gin`. See
`docs/superpowers/specs/2026-09-25-react-agent-frontend-design.md` for the design.

## Run locally

Start the API first (see `../gin/README.md`; it listens on :8080), then:

    npm install
    npm run dev          # http://localhost:5173, proxies /api to :8080

If port 5173 is already in use, override it: `npm run dev -- --port <n>`.

Sign in with an account created by `go run ./cmd/api create-admin` or `POST /staff`.

## Screens

The UI is a re-skin in the style of the osTicket staff control panel (spec:
`docs/superpowers/specs/2026-09-26-scp-skin-design.md`). A header with the panel switch and
**Log Out** sits above a tab bar and a per-tab sub-nav.

- **Agent Panel** tabs:
  - **Dashboard** (`/dashboard`): ticket activity line chart and department / topic / staff
    breakdowns for a chosen period (7, 14, 30 or 90 days ending today by default), from
    `GET /api/v1/dashboard/stats`.
  - **Tickets** (`/tickets`): the queue, with a sub-nav of Open, My Tickets, Unassigned,
    Closed and All, plus **New Ticket** (`/tickets/new`). The ticket view (`/tickets/:id`) has
    inline edits on its info tables and a tabbed Post Reply / Post Internal Note composer.
- **Admin Panel** (admin staff only; `/admin` opens Departments) tabs: Dashboard,
  **Departments**, **Help Topics**, **Staff**, and **Email** with its own sub-nav of Templates
  (`/admin/email/templates`, edit at `/admin/email/templates/:key`), Outbox (a message's
  subject opens `/admin/email/outbox/:id` with its headers, text body and HTML source) and
  Inbound Log.
  Each list has one green "Add New ..." button in its sticky bar; records open in a form
  (`/admin/<section>/new`, `/admin/<section>/<id>`). Departments and topics can be deleted
  from the list (the API refuses while something still references them); staff are
  deactivated instead, and the staff form has a "Set password" section.

## Customer portal

A second front door for customers, in the style of osTicket's client portal (spec:
`docs/superpowers/specs/2026-09-26-customer-portal-design.md`). It is mounted at `/portal/*`
outside the staff sign-in, with its own `PortalAuthProvider` and `PortalShell`, and talks to
`/api/v1/portal` only.

| Route | Page |
|---|---|
| `/portal` | landing: Open a New Ticket, Check Ticket Status |
| `/portal/open`, `/portal/opened/:number` | open a ticket (anonymous or signed in) and its confirmation |
| `/portal/login` | password or emailed sign-in link on the left; guest access (email + ticket number) on the right |
| `/portal/register`, `/portal/reset` | create an account (name and email; the emailed confirm link sets the password), request a reset link |
| `/portal/t/:token` | redeems an emailed link and routes by its kind: access → the ticket, signin → My Tickets, confirm/reset → `/portal/profile#password` |
| `/portal/tickets/:id` | the ticket: thread (messages and agent responses only), reply with attachments, Close / Reopen |
| `/portal/tickets`, `/portal/profile` | My Tickets (Open / Closed) and the profile; account sessions only |

- **Session storage**: the portal keeps its refresh token under `ticket.portal_refresh_token`
  and never reads or writes the staff key `ticket.refresh_token`, so a browser can hold both.
  Confirm and reset sessions have no refresh token and do not survive a reload.
- **Guest sessions** come from an access link: they are scoped to one ticket (view, reply,
  close, reopen, download) and are sent to that ticket from account-only pages.
- **Attachments**: portal uploads return an access `token` beside the file `id`; the open and
  reply forms send `file_tokens` index-for-index with `file_ids`.
- **Links in mail**: every portal email carries `${APP_BASE_URL}/portal/t/<token>`. The e2e
  walk does not need a real mailbox: it reads the link from the queued mail's text body on the
  admin outbox message page.

## Design tokens

Colours, fonts, radii, spacing and the content width live as CSS variables in
`src/styles/tokens.css`; `src/styles/global.css` holds the element defaults. CSS modules must
use those variables: `src/styles/tokens.test.ts` (part of `npm test`) fails any `*.module.css`
that contains a colour literal, a `font-family` other than `var(...)`, or a px
`border-radius`. Shared building blocks live in `src/ui`.

## Scripts

    npm test             # Vitest + Testing Library + MSW, headless
    npm run lint
    npm run build        # type-check and bundle to dist/

In production, serve `dist/` and the API's `/api` from the same origin (or behind one reverse
proxy), with SPA history fallback to `index.html`; the `/api` proxy exists only under `npm run dev`.

## End-to-end walk

`e2e/walk.mjs` drives a real browser through sign-in, the queue, the dashboard, a new ticket,
an internal note, an admin form and the email templates, then a customer portal leg (open a
ticket anonymously, request a guest access link, follow it, reply and close), saving ten
screenshots. The portal leg needs the API running with mail enabled so the access mail is
queued (see `e2e/README.md`). It has its own `package.json` so Playwright stays out of the
app's dependencies. With the API and `npm run dev` running:

    cd e2e && npm install && npx playwright install chromium
    E2E_USER=<admin username> E2E_PASS=<password> node walk.mjs

See `e2e/README.md` for the variables.

## Layout

`src/api` is the only code that talks to the network (typed client with refresh-on-401).
`src/auth` owns the staff session and `src/portal` the customer portal (its own session,
shell and pages). `src/pages` are the staff routes; `src/ui` holds the shared layout
primitives and `src/components` the ticket-view pieces;
`src/hooks` wrap TanStack Query and URL state.
