INSERT INTO ticket_priority (name, urgency, color) VALUES
  ('low', 1, '#DDFFDD'),
  ('normal', 2, '#FFFFFF'),
  ('high', 3, '#FEE7E7'),
  ('emergency', 4, '#FF0000');

INSERT INTO ticket_status (name, state, sort_order) VALUES
  ('Open', 'open', 1),
  ('Resolved', 'resolved', 2),
  ('Closed', 'closed', 3);

INSERT INTO department (name, is_public) VALUES ('Support', true);

INSERT INTO help_topic (name, dept_id, priority_id, is_active, sort_order) VALUES (
  'General Inquiry',
  (SELECT id FROM department WHERE name = 'Support'),
  (SELECT id FROM ticket_priority WHERE name = 'normal'),
  true, 1
);
