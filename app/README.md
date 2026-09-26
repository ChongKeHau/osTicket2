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
  (`/admin/email/templates`, edit at `/admin/email/templates/:key`), Outbox and Inbound Log.
  Each list has one green "Add New ..." button in its sticky bar; records open in a form
  (`/admin/<section>/new`, `/admin/<section>/<id>`). Departments and topics can be deleted
  from the list (the API refuses while something still references them); staff are
  deactivated instead, and the staff form has a "Set password" section.

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
an internal note, an admin form and the email templates, saving six screenshots. It has its
own `package.json` so Playwright stays out of the app's dependencies. With the API and
`npm run dev` running:

    cd e2e && npm install && npx playwright install chromium
    E2E_USER=<admin username> E2E_PASS=<password> node walk.mjs

See `e2e/README.md` for the variables.

## Layout

`src/api` is the only code that talks to the network (typed client with refresh-on-401).
`src/auth` owns the session. `src/pages` are routes; `src/ui` holds the shared layout
primitives and `src/components` the ticket-view pieces;
`src/hooks` wrap TanStack Query and URL state.
