ALTER TYPE ticket_source ADD VALUE 'email';

CREATE TABLE email_template (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key        text NOT NULL UNIQUE,
  subject    text NOT NULL,
  body_html  text NOT NULL,
  body_text  text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TYPE email_status AS ENUM ('pending', 'sent', 'failed');

CREATE TABLE email_outbox (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ticket_id       bigint NOT NULL REFERENCES ticket(id) ON DELETE CASCADE,
  entry_id        bigint REFERENCES thread_entry(id) ON DELETE SET NULL,
  template_key    text NOT NULL,
  to_address      text NOT NULL,
  to_name         text NOT NULL DEFAULT '',
  subject         text NOT NULL,
  body_html       text NOT NULL,
  body_text       text NOT NULL,
  message_id      text NOT NULL UNIQUE,
  in_reply_to     text,
  auto_submitted  boolean NOT NULL DEFAULT false,
  status          email_status NOT NULL DEFAULT 'pending',
  attempts        int NOT NULL DEFAULT 0,
  last_error      text,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  sent_at         timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_outbox_pending_idx ON email_outbox (next_attempt_at) WHERE status = 'pending';
CREATE INDEX email_outbox_ticket_idx ON email_outbox (ticket_id, to_address, sent_at);

CREATE TYPE inbound_outcome AS ENUM ('created', 'replied', 'ignored');

CREATE TABLE inbound_message (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  message_id   text NOT NULL UNIQUE,
  from_address text NOT NULL,
  from_name    text NOT NULL DEFAULT '',
  subject      text NOT NULL DEFAULT '',
  ticket_id    bigint REFERENCES ticket(id) ON DELETE SET NULL,
  entry_id     bigint REFERENCES thread_entry(id) ON DELETE SET NULL,
  outcome      inbound_outcome NOT NULL,
  reason       text NOT NULL DEFAULT '',
  received_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO email_template (key, subject, body_html, body_text) VALUES
('ticket_autoresp',
 '[#{{.Number}}] {{.Subject}}',
 '<p>Hello {{.RequesterName}},</p><p>Thank you for contacting {{.SiteName}}. Your request has been received and a ticket has been opened.</p><p><strong>Ticket #{{.Number}}</strong>: {{.Subject}}</p><p>You can reply to this email to add information to your ticket.</p><p><a href="{{.Link}}">{{.Link}}</a></p><hr>{{.MessageHTML}}',
 'Hello {{.RequesterName}},

Thank you for contacting {{.SiteName}}. Your request has been received and a ticket has been opened.

Ticket #{{.Number}}: {{.Subject}}

You can reply to this email to add information to your ticket.
{{.Link}}

----
{{.Message}}'),
('ticket_reply',
 '[#{{.Number}}] {{.Subject}}',
 '<p>Hello {{.RequesterName}},</p><p>{{.AgentName}} replied to your ticket <strong>#{{.Number}}</strong>:</p><blockquote>{{.MessageHTML}}</blockquote><p>Reply to this email to continue the conversation, or view the ticket at <a href="{{.Link}}">{{.Link}}</a>.</p>',
 'Hello {{.RequesterName}},

{{.AgentName}} replied to your ticket #{{.Number}}:

{{.Message}}

Reply to this email to continue the conversation, or view the ticket at {{.Link}}'),
('assigned_alert',
 '[#{{.Number}}] Assigned to you: {{.Subject}}',
 '<p>Hello {{.AgentName}},</p><p>Ticket <strong>#{{.Number}}</strong> ({{.Subject}}) from {{.RequesterName}} &lt;{{.RequesterEmail}}&gt; has been assigned to you.</p><p><a href="{{.Link}}">{{.Link}}</a></p>',
 'Hello {{.AgentName}},

Ticket #{{.Number}} ({{.Subject}}) from {{.RequesterName}} <{{.RequesterEmail}}> has been assigned to you.

{{.Link}}'),
('message_alert',
 '[#{{.Number}}] New message: {{.Subject}}',
 '<p>{{.RequesterName}} &lt;{{.RequesterEmail}}&gt; wrote on ticket <strong>#{{.Number}}</strong>:</p><blockquote>{{.MessageHTML}}</blockquote><p><a href="{{.Link}}">{{.Link}}</a></p>',
 '{{.RequesterName}} <{{.RequesterEmail}}> wrote on ticket #{{.Number}}:

{{.Message}}

{{.Link}}');
