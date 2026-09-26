# Customer portal — design

Date: 2026-09-26. Branch: `portal` from `main` (6bd2ceda). Follows the staff-panel re-skin.

## 1. Purpose

End users have no way in today: tickets carry a requester name and email, every API route is
staff-only, and the React app has a single staff session. This slice adds the customer side
of the desk, modelled on osTicket's client portal: a landing page, open a ticket without an
account, sign in with a password or an emailed link, guest access to one ticket by number
and email, a list of your own tickets, the thread with replies and attachments, close and
reopen, a profile with password management. It lives in the existing React app under
`/portal` with its own shell and session, and in the Go API under `/api/v1/portal` with its
own identity model, so customer and agent identity never mix.

Decisions from brainstorming: password accounts with email confirmation **and**
passwordless sign-in links **and** per-ticket guest access; scope is core ticketing plus
close/reopen; same app with `/portal` routes; osTicket's client-portal structure with the
existing modern token skin.

## 2. Data model (migration `V5__portal.sql`)

- `end_user`: `id`, `email` (unique on `lower(email)`), `name`, `password_hash` (nullable;
  null until the user sets one), `email_verified_at` (nullable), `created_at`,
  `updated_at`. A row is created or matched (case-insensitively) the first time an address
  opens a ticket, requests a sign-in link, requests guest access, or registers.
- `client_token`: `id`, `end_user_id`, `kind` enum `client_token_kind`
  (`confirm`, `reset`, `signin`, `access`), `token_hash` (SHA-256 of a 32-byte random
  token; the raw token exists only in the emailed URL), `ticket_id` (nullable; set for
  `access`), `expires_at`, `used_at` (nullable), `created_at`. One-time use. Lifetimes:
  `confirm` and `reset` 24 h, `signin` and `access` 1 h. Index on `(end_user_id, kind)`.
- `client_refresh_token`: same columns as `refresh_token` with `end_user_id` in place of
  `staff_id`.
- `ticket.user_id` (nullable FK to `end_user`, `ON DELETE SET NULL`), backfilled in the
  migration by creating one `end_user` per distinct `lower(requester_email)` (name from the
  most recent ticket) and linking every ticket. Every new ticket sets it.
- `thread_entry.user_id` (nullable FK), set when a customer posts a message.
- `file.access_token` (nullable text; amended during execution): a random token set on
  portal uploads and returned to the uploader, who must present it to attach the file (§4).
  Staff and inbound-mail files have none.
- `email_outbox.ticket_id` becomes nullable (amended): account mail (confirm, sign-in and
  reset links) belongs to no ticket. The outbox JSON carries `ticket_id: null` for those rows
  and the staff Outbox page shows "—".
- `ticket_event.data` carries `{"user_id": n}` for customer-initiated `status_changed`
  events; no new event kinds.
- Seeded `email_template` rows: `client_confirm`, `client_signin_link`,
  `client_access_link`, `client_reset` (subject, HTML and text bodies using `.Link`,
  `.SiteName`, `.RequesterName`, and `.Number`/`.Subject` for access links).

## 3. Identity and sessions (package `gin/internal/client`)

Mirrors `internal/auth` but is a separate package with separate tables and a separate
principal.

- **JWT**: same signing secret, `aud: "client"`, `sub` = end user id, 15-minute access
  token; refresh tokens rotate on use exactly like the staff ones, stored hashed in
  `client_refresh_token`. The staff middleware rejects tokens with `aud: client`; the client
  middleware rejects tokens without it. A guest session's access token also carries
  `tid` (the ticket id) and its refresh token is issued with the same scope.
- **Principal**: `client.Principal{UserID int64, Email string, Verified bool, TicketID *int64}`;
  `TicketID` non-nil means a guest session limited to that ticket.
- **Middleware**: `client.RequireUser()` (any session) and `client.RequireAccount()` (rejects
  guest sessions with 403 code `guest_session`).
- **Flows** (all under `/api/v1/portal/auth`):
  - `POST login` `{email, password}` → session. Requires a password and
    `email_verified_at`; otherwise 401 with the generic message. Rate-limited by the
    existing fixed-window limiter keyed by `lower(email)` and by client IP.
  - `POST link` `{email}` → 202 always. If the address exists, creates a `signin` token and
    enqueues `client_signin_link`; if not, does nothing. Rate-limited per email and IP.
  - `POST exchange` `{token}` → session. Looks up the hash, requires `used_at IS NULL` and
    not expired, marks used, and for `confirm` sets `email_verified_at`; for `access`
    returns a guest session scoped to `ticket_id`; for `reset` returns a short-lived
    session flagged `reset` whose only permitted call is `POST me/password`. Failures
    return 410 `token_invalid`.
  - `POST register` `{email, name}` → 201 always (amended during execution: no password
    and no 409 `exists`). It creates or matches the end user and enqueues `client_confirm`,
    or does nothing when the address already has a password. The confirm exchange marks the
    email verified and returns a password-setting session (like `reset`, `kind: "confirm"`),
    and the person who clicked sets the password with `POST me/password` — so nobody can
    pre-register someone else's address with their own password. Password policy: at least
    8 characters, hashed with the existing helper.
  - `POST reset` `{email}` → 202 always; enqueues `client_reset` for existing users.
  - `link`, `reset`, `access` and `register` validate and rate-limit synchronously, answer
    202/201 at once, and do the lookup, token and mail work in the background (amended), so
    response timing does not reveal whether an address or ticket exists.
  - `POST refresh` `{refresh_token}` and `POST logout` mirror the staff endpoints.
- **Guest access**: `POST /api/v1/portal/access` `{email, number}` → 202 always. If a ticket
  with that number exists and `lower(requester_email)` matches, creates an `access` token
  scoped to it and enqueues `client_access_link`. Rate-limited per email and IP.

## 4. Portal API

Public (no session):
- `GET /api/v1/portal/reference` → `{site_name, departments:[{id,name}] (public only),
  topics:[{id,name}] (active only)}`.
- `POST /api/v1/portal/tickets` `{name, email, subject, message, format, topic_id?,
  dept_id?, file_ids?, file_tokens?}` → 201 `{id, number}`. Creates or matches the end user, calls the
  existing `CreateExternal` with source `web` (so the autoresponse is enqueued) and sets
  `ticket.user_id`. When a session is present the email is taken from the session, not
  the body. Rate-limited per IP and per email at 10 per hour.
- `POST /api/v1/portal/files` → same as the staff upload but allowed for portal sessions
  and for anonymous callers with the same per-IP limit; anonymous uploads expire unused
  after 24 h via the existing file GC. The response also carries `token`, a random access
  token stored as `file.access_token` (amended): ticket creation and replies attach a file
  only when the caller sends its token in `file_tokens`, at the same index as the id in
  `file_ids`, so a guessed file id alone cannot claim someone else's upload. Staff uploads
  have no token and cannot be attached from the portal.

Signed in (`RequireUser`, guest sessions limited to their ticket):
- `GET tickets?state=open|closed&page&page_size` → own tickets (`user_id = me`), sorted by
  `last_message_at desc`; guests get 403 `guest_session`.
- `GET tickets/:id` → ticket (number, subject, status name and state, department, topic,
  created, last message, closed) plus thread entries of type `message` and `response`
  only, each with poster name (customer name or agent display name), body (sanitised HTML
  or text), created, attachments (name, size, download URL). Others' tickets → 404.
- `GET tickets/:id/files/:fileId` → attachment download, checked against the ticket.
- `POST tickets/:id/reply` `{body, format, file_ids?, file_tokens?}` → appends a `message` entry with
  `user_id` through the existing `AppendMessage` (unanswered flag, agent alert). If the
  ticket is in a `closed` state it is first moved to the first `open`-state status and a
  `status_changed` event is recorded.
- `POST tickets/:id/close` → sets the first `closed`-state status (409 if already closed);
  `POST tickets/:id/reopen` → first `open`-state status (409 if not closed). Both record
  `status_changed` with `{"user_id": me}`.
- `GET me` → `{id, email, name, verified, has_password}`; `PATCH me` `{name}`;
  `POST me/password` `{password, current_password?}` (current required when one exists;
  a `reset` session may omit it; setting a password marks the email verified).

Errors use the shared envelope and status mapping; 410 `token_invalid` for token exchange;
429 with `retry_after` seconds for rate limits. Sign-in link, reset and guest access never
reveal whether an address or ticket exists.

## 5. Portal frontend (`app/src/portal/`)

Mounted in `App.tsx` as a subtree at `/portal/*` outside the staff `RequireAuth`, with its
own `PortalAuthProvider` (session store `api/portalClient.ts` using storage key
`ticket.portal_refresh_token`, separate from the staff key) and `PortalShell`.

- **Shell**: header with the site name and the user bar ("Guest User · Sign In", or
  "Name · Profile · Tickets · Sign Out", or for a guest session "Ticket #n · Sign Out"),
  nav tabs **Support Center Home** | **Open a New Ticket** | **Check Ticket Status**,
  centred content panel on the page background, footer. Tokens and primitives from
  `src/ui` (`Banner`, `Button`, `FormTable`, `FormActions`, `ListTable`, `Pagination`,
  `SubNav`, `TabBar`, `Tabs`); no new dependencies.
- `/portal` landing: welcome paragraph and two large buttons, Open a New Ticket and Check
  Ticket Status, as osTicket's landing page.
- `/portal/open`: `FormTable` sections Contact (Name, Email; prefilled and read-only when
  signed in) and Ticket (Help Topic, Department if any public, Subject, Message,
  Attachments). Success page shows the ticket number; anonymous users also read "We emailed
  you a link to follow this ticket" (the autoresponse carries the ticket number; the guest
  form gets them in).
- `/portal/login`: two columns. Left "Sign in": Email, Password, Sign In, links "Email me a
  sign-in link" (submits the email only) and "Forgot password". Right "Check a ticket as a
  guest": Email, Ticket Number, "Email me an access link", and "Create an account".
- `/portal/register`: Name and Email only; then a "Check your email" page. The password is set
  after the confirm link is followed.
- `/portal/t/:token`: single token page that calls `exchange`, stores the session, and
  redirects by kind: `confirm` → `/portal/profile#password` with an "Email confirmed — set your
  password" flash; `signin` →
  tickets; `access` → that ticket; `reset` → `/portal/profile#password`. Failure shows a
  banner "This link has expired or was already used" with buttons to request a new one.
- `/portal/reset`: Email → "Check your email".
- `/portal/tickets`: `ListTable` (Number, Date, Subject, Department, Status, Last Message)
  with Open | Closed sub-nav and pagination; guest sessions are redirected to their ticket.
- `/portal/tickets/:id`: info table (Number, Status, Department, Help Topic, Created, Last
  Updated), the thread (customer messages left-aligned, agent responses right-aligned,
  attachments listed), Close / Reopen button per state, reply form (message, attachments,
  Post Reply) with a note that replying reopens a closed ticket.
- `/portal/profile`: Name; Password section (Current password when one exists, New,
  Confirm; a link-only account sees "Set a password").
- Route guards: `RequirePortalUser` redirects anonymous visitors to `/portal/login` with
  `from`; `RequirePortalAccount` sends guests to their ticket.

Staff app: the thread shows customer messages with the customer's name and the ticket
view's Source reads `web` for portal tickets; the admin templates page lists the four new
keys; nothing else changes.

## 6. Error handling

Same envelope and mapping as the rest of the API. Token pages map 410 to the expired-link
banner with "request a new link" actions. Guest sessions receive 403 `guest_session` on
account-only routes and the pages redirect them to their ticket. 429 responses show
"Too many attempts, try again in N minutes". Uploads that exceed the size or type limits
show the field error under Attachments. A reply to a ticket that was deleted or moved out
of reach shows 404 and returns to the list.

## 7. Testing

- Go (`internal/client`, testcontainers Postgres): register → confirm → password login;
  link request → exchange → session; access link scoped to one ticket and refused on
  another; reset → set password → verified; tokens single-use and expiring; audience
  separation (staff token rejected by client middleware and vice versa); refresh rotation;
  backfill creates one user per address and links tickets; own-tickets listing and 404 on
  others'; reply reopens a closed ticket and records the event; close/reopen transitions
  and 409s; rate limits; the no-disclosure rule (identical responses for known and unknown
  addresses). Handler tests with fakes for status codes. `make test` above 75%.
- React (Vitest + msw): every portal page; token page redirects per kind and the failure
  banner; guest redirect; login two-column behaviours; open-ticket success states; the
  portal session store never reads or writes the staff key; token lint unchanged.
- End-to-end: the Playwright walk gains a portal leg: open a ticket anonymously, request a
  guest access link, read the raw link from the outbox row through the admin outbox API
  (so no real mail is needed), follow it, reply, close, then sign in as an agent and see
  the customer's message and the `web` source.

## 8. Out of scope

Organizations and org-wide ticket visibility, collaborators/CC, CAPTCHA, email address
changes, external or social login, knowledge base on the portal, custom forms, ticket
editing by the customer, per-user timezone and language, portal branding beyond the site
name, deleting accounts.
