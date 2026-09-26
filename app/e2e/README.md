# End-to-end walk

`walk.mjs` is a Playwright script (not a test suite) that signs in and walks the main screens
of the staff panel against a running stack, then walks the customer portal as a guest, saving
one full-page screenshot per stop:

| Shot | Screen |
|---|---|
| `01-queue` | ticket queue after sign-in |
| `02-dashboard` | dashboard ("Ticket Activity") |
| `03-ticket` | a ticket just opened from New Ticket |
| `04-ticket-note` | the same ticket after posting an internal note |
| `05-admin-form` | the first department's form in the Admin Panel |
| `06-email-templates` | Admin Panel, Email, Templates |
| `07-portal-landing` | customer portal home, signed out |
| `08-portal-opened` | "Ticket #n opened" after an anonymous open |
| `09-portal-ticket` | the ticket, reached through the emailed guest access link |
| `10-portal-closed` | the same ticket after a customer reply and Close ticket → Confirm |

Each run creates two tickets (requesters `walk@example.test` and `walk-customer@example.test`),
one internal note, one customer reply and a few outbox rows, so point it at a development
database.

## The portal leg

The customer uses a second browser context, so it shares no storage with the staff session.
It opens a ticket anonymously (choosing the first department), asks for a guest access link on
`/portal/login`, and then the still-signed-in admin page opens Admin Panel → Email → Outbox,
reloading until the row `[#n] Access link for Portal walk ticket` appears (the API queues
account mail in the background after answering 202). The row's subject opens the message
page, whose first `<pre>` is the text body; the walk takes the `…/portal/t/<token>` link from
it and follows it as the customer, then replies and closes the ticket.

So the API must run with mail **enabled** (a disabled notifier queues nothing) and
`APP_BASE_URL` set to the app's origin. No mail has to be delivered: point SMTP and IMAP at a
closed local port and the rows stay `pending` while the sender retries, e.g.

    MAIL_ENABLED=true MAIL_FROM="Desk <desk@example.test>" SMTP_PASSWORD=x \
    SMTP_HOST=127.0.0.1 SMTP_PORT=2525 SMTP_TLS=none \
    IMAP_HOST=127.0.0.1 IMAP_PORT=1143 IMAP_TLS=none \
    APP_BASE_URL=http://localhost:5173 go run ./cmd/api

(the sender and poller log connection errors; that is expected). Anonymous opens are limited
to 10 an hour per IP, so more than ten runs an hour against one API process will fail at
`08-portal-opened` until the window passes or the API restarts.

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
