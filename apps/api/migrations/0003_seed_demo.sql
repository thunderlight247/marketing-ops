-- Initial seed data.
-- Creates the default operator if not already present.

INSERT INTO operators (name, email)
VALUES ('Operator', 'operator@local.example')
ON CONFLICT (email) DO NOTHING;

