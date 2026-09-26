CREATE TYPE client_token_kind AS ENUM ('confirm', 'reset', 'signin', 'access');

CREATE TABLE end_user (
  id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  email              text NOT NULL,
  name               text NOT NULL DEFAULT '',
  password_hash      text,
  email_verified_at  timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX end_user_email_idx ON end_user (lower(email));

CREATE TABLE client_token (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  end_user_id  bigint NOT NULL REFERENCES end_user(id) ON DELETE CASCADE,
  kind         client_token_kind NOT NULL,
  token_hash   text NOT NULL UNIQUE,
  ticket_id    bigint REFERENCES ticket(id) ON DELETE CASCADE,
  expires_at   timestamptz NOT NULL,
  used_at      timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX client_token_user_kind_idx ON client_token (end_user_id, kind);

CREATE TABLE client_refresh_token (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  token_hash   text NOT NULL UNIQUE,
  end_user_id  bigint NOT NULL REFERENCES end_user(id) ON DELETE CASCADE,
  ticket_id    bigint REFERENCES ticket(id) ON DELETE CASCADE,
  expires_at   timestamptz NOT NULL,
  revoked_at   timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX client_refresh_token_user_idx ON client_refresh_token (end_user_id);

ALTER TABLE ticket ADD COLUMN user_id bigint REFERENCES end_user(id) ON DELETE SET NULL;
CREATE INDEX ticket_user_idx ON ticket (user_id, last_message_at DESC);
ALTER TABLE thread_entry ADD COLUMN user_id bigint REFERENCES end_user(id) ON DELETE SET NULL;

-- Backfill: one end user per distinct address, named from the most recent ticket.
-- NOT EXISTS guards this so re-running the migration's statements (as the schema
-- test does, to prove idempotency) never creates duplicate end users.
INSERT INTO end_user (email, name)
SELECT DISTINCT ON (lower(requester_email)) requester_email, requester_name
FROM ticket
WHERE requester_email <> ''
  AND NOT EXISTS (SELECT 1 FROM end_user u WHERE lower(u.email) = lower(ticket.requester_email))
ORDER BY lower(requester_email), created_at DESC, id DESC;

UPDATE ticket t SET user_id = u.id
FROM end_user u WHERE lower(u.email) = lower(t.requester_email);

ALTER TABLE email_outbox ALTER COLUMN ticket_id DROP NOT NULL;

INSERT INTO email_template (key, subject, body_html, body_text) VALUES
('client_confirm', 'Confirm your {{.SiteName}} account',
 '<p>Hello {{.RequesterName}},</p><p>Confirm your email address to finish creating your {{.SiteName}} account:</p><p><a href="{{.Link}}">{{.Link}}</a></p><p>This link expires in 24 hours. If you did not create an account, ignore this message.</p>',
 E'Hello {{.RequesterName}},\n\nConfirm your email address to finish creating your {{.SiteName}} account:\n\n{{.Link}}\n\nThis link expires in 24 hours. If you did not create an account, ignore this message.'),
('client_signin_link', 'Your {{.SiteName}} sign-in link',
 '<p>Hello {{.RequesterName}},</p><p>Use this link to sign in to {{.SiteName}}:</p><p><a href="{{.Link}}">{{.Link}}</a></p><p>It expires in 1 hour and works once. If you did not request it, ignore this message.</p>',
 E'Hello {{.RequesterName}},\n\nUse this link to sign in to {{.SiteName}}:\n\n{{.Link}}\n\nIt expires in 1 hour and works once. If you did not request it, ignore this message.'),
('client_access_link', '[#{{.Number}}] Access link for {{.Subject}}',
 '<p>Hello {{.RequesterName}},</p><p>Use this link to view ticket #{{.Number}} ({{.Subject}}):</p><p><a href="{{.Link}}">{{.Link}}</a></p><p>It expires in 1 hour and works once.</p>',
 E'Hello {{.RequesterName}},\n\nUse this link to view ticket #{{.Number}} ({{.Subject}}):\n\n{{.Link}}\n\nIt expires in 1 hour and works once.'),
('client_reset', 'Reset your {{.SiteName}} password',
 '<p>Hello {{.RequesterName}},</p><p>Use this link to set a new password for {{.SiteName}}:</p><p><a href="{{.Link}}">{{.Link}}</a></p><p>It expires in 24 hours and works once. If you did not request it, ignore this message.</p>',
 E'Hello {{.RequesterName}},\n\nUse this link to set a new password for {{.SiteName}}:\n\n{{.Link}}\n\nIt expires in 24 hours and works once. If you did not request it, ignore this message.');
