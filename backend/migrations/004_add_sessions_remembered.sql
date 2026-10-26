-- Remembered sessions (issue 379): a login that opted in via the
-- login request's rememberDevice flag lives on the longer remembered
-- TTL (SESSION_REMEMBER_TTL_DAYS). The flag is server-side truth on
-- the session row; refresh rotation copies it onto the replacement
-- row. Rows predating the column are unremembered by default.
ALTER TABLE sessions ADD COLUMN remembered BOOLEAN NOT NULL DEFAULT 0;
