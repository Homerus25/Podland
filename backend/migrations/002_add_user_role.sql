-- Add role column to users table.
-- Existing rows keep working unchanged: they default to 'user'.
-- Role values are constrained to {user, admin} at the application
-- boundary (see internal/models/user_repository.go) because SQLite
-- cannot ALTER a CHECK constraint onto an existing table.
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user';
