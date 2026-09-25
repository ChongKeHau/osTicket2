CREATE TYPE ticket_state AS ENUM ('open', 'resolved', 'closed');
CREATE TYPE ticket_source AS ENUM ('web', 'api', 'phone', 'other');
CREATE TYPE thread_entry_type AS ENUM ('message', 'response', 'note');
CREATE TYPE body_format AS ENUM ('html', 'text');
CREATE TYPE ticket_event_kind AS ENUM (
  'created', 'assigned', 'unassigned', 'status_changed', 'transferred', 'closed', 'reopened', 'edited'
);

CREATE TABLE department (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name        text NOT NULL UNIQUE,
  is_public   boolean NOT NULL DEFAULT true,
  manager_id  bigint,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE staff (
  id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  username         text NOT NULL UNIQUE,
  email            text NOT NULL UNIQUE,
  password_hash    text NOT NULL,
  first_name       text NOT NULL DEFAULT '',
  last_name        text NOT NULL DEFAULT '',
  is_admin         boolean NOT NULL DEFAULT false,
  is_active        boolean NOT NULL DEFAULT true,
  primary_dept_id  bigint NOT NULL REFERENCES department(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE department
  ADD CONSTRAINT department_manager_fk FOREIGN KEY (manager_id) REFERENCES staff(id);

CREATE TABLE staff_department (
  staff_id    bigint NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
  dept_id     bigint NOT NULL REFERENCES department(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (staff_id, dept_id)
);

CREATE TABLE refresh_token (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  token_hash  text NOT NULL UNIQUE,
  staff_id    bigint NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
  expires_at  timestamptz NOT NULL,
  revoked_at  timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_token_staff_idx ON refresh_token (staff_id);

CREATE TABLE ticket_priority (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name        text NOT NULL UNIQUE,
  urgency     int NOT NULL,
  color       text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ticket_status (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name        text NOT NULL UNIQUE,
  state       ticket_state NOT NULL,
  sort_order  int NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE help_topic (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name         text NOT NULL,
  dept_id      bigint REFERENCES department(id),
  priority_id  bigint REFERENCES ticket_priority(id),
  is_active    boolean NOT NULL DEFAULT true,
  sort_order   int NOT NULL DEFAULT 0,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE SEQUENCE ticket_number_seq START 1;

CREATE TABLE ticket (
  id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  number             text NOT NULL UNIQUE,
  subject            text NOT NULL,
  status_id          bigint NOT NULL REFERENCES ticket_status(id),
  dept_id            bigint NOT NULL REFERENCES department(id),
  topic_id           bigint REFERENCES help_topic(id),
  priority_id        bigint NOT NULL REFERENCES ticket_priority(id),
  assigned_staff_id  bigint REFERENCES staff(id),
  requester_name     text NOT NULL DEFAULT '',
  requester_email    text NOT NULL,
  source             ticket_source NOT NULL DEFAULT 'web',
  is_answered        boolean NOT NULL DEFAULT false,
  due_at             timestamptz,
  closed_at          timestamptz,
  last_message_at    timestamptz NOT NULL DEFAULT now(),
  last_response_at   timestamptz,
  extra              jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ticket_status_idx ON ticket (status_id);
CREATE INDEX ticket_dept_idx ON ticket (dept_id);
CREATE INDEX ticket_assignee_idx ON ticket (assigned_staff_id);
CREATE INDEX ticket_last_message_idx ON ticket (last_message_at DESC);
CREATE INDEX ticket_subject_search_idx ON ticket USING GIN (to_tsvector('english', subject));

CREATE TABLE thread_entry (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ticket_id   bigint NOT NULL REFERENCES ticket(id) ON DELETE CASCADE,
  type        thread_entry_type NOT NULL,
  staff_id    bigint REFERENCES staff(id),
  poster      text NOT NULL DEFAULT '',
  title       text,
  body        text NOT NULL,
  format      body_format NOT NULL DEFAULT 'html',
  parent_id   bigint REFERENCES thread_entry(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX thread_entry_ticket_idx ON thread_entry (ticket_id, id);

CREATE TABLE ticket_event (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ticket_id   bigint NOT NULL REFERENCES ticket(id) ON DELETE CASCADE,
  staff_id    bigint REFERENCES staff(id),
  kind        ticket_event_kind NOT NULL,
  data        jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ticket_event_ticket_idx ON ticket_event (ticket_id, id);

CREATE TABLE file (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key          text NOT NULL UNIQUE,
  name         text NOT NULL,
  mime         text NOT NULL,
  size         bigint NOT NULL,
  sha256       text NOT NULL,
  backend      text NOT NULL DEFAULT 'local',
  uploaded_by  bigint REFERENCES staff(id),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE attachment (
  thread_entry_id  bigint NOT NULL REFERENCES thread_entry(id) ON DELETE CASCADE,
  file_id          bigint NOT NULL REFERENCES file(id),
  inline           boolean NOT NULL DEFAULT false,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (thread_entry_id, file_id)
);
CREATE UNIQUE INDEX attachment_file_idx ON attachment (file_id);
