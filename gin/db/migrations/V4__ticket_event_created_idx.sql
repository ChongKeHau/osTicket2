-- Dashboard stats (db/queries/dashboard.sql) filter ticket_event by a created_at window.
CREATE INDEX ticket_event_created_idx ON ticket_event (created_at);
