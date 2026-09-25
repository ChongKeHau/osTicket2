# Ticket Desk (React frontend)

Agent UI for the Go ticket API in `../gin`. See
`docs/superpowers/specs/2026-09-25-react-agent-frontend-design.md` for the design.

## Run locally

Start the API first (see `../gin/README.md`; it listens on :8080), then:

    npm install
    npm run dev          # http://localhost:5173, proxies /api to :8080

If port 5173 is already in use, override it: `npm run dev -- --port <n>`.

Sign in with an account created by `go run ./cmd/api create-admin` or `POST /staff`.

## Scripts

    npm test             # Vitest + Testing Library + MSW, headless
    npm run lint
    npm run build        # type-check and bundle to dist/

In production, serve `dist/` and the API's `/api` from the same origin (or behind one reverse
proxy), with SPA history fallback to `index.html`; the `/api` proxy exists only under `npm run dev`.

## Layout

`src/api` is the only code that talks to the network (typed client with refresh-on-401).
`src/auth` owns the session. `src/pages` are routes; `src/components` are shared pieces;
`src/hooks` wrap TanStack Query and URL state.
