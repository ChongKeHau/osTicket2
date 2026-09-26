# End-to-end walk

`walk.mjs` is a Playwright script (not a test suite) that signs in and walks the main screens
of the staff panel against a running stack, saving one full-page screenshot per stop:

| Shot | Screen |
|---|---|
| `01-queue` | ticket queue after sign-in |
| `02-dashboard` | dashboard ("Ticket Activity") |
| `03-ticket` | a ticket just opened from New Ticket |
| `04-ticket-note` | the same ticket after posting an internal note |
| `05-admin-form` | the first department's form in the Admin Panel |
| `06-email-templates` | Admin Panel, Email, Templates |

It creates one ticket (requester `walk@example.test`) and one internal note each run, so point
it at a development database.

## Run

Start the API (`../../gin/README.md`, on :8080) and the dev server (`npm run dev` in `app/`,
on :5173, proxying `/api`), then:

    npm install
    npx playwright install chromium    # first run only
    E2E_USER=<admin username> E2E_PASS=<password> node walk.mjs

| Variable | Default | Notes |
|---|---|---|
| `APP_URL` | `http://localhost:5173` | the app's dev server (or any origin serving the app with `/api`) |
| `E2E_USER` | `admin` | an admin account, e.g. one made with `api create-admin` |
| `E2E_PASS` | `admin1234` | its password |

Screenshots go to `shots/`, which is git-ignored (as is `node_modules/`). Playwright is a dev
dependency of this folder only; the app's own `package.json` does not include it.
