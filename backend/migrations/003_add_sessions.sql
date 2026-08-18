-- Sessions: one row per login, backing revocable refresh sessions.
-- Only the SHA-256 hex hash of the opaque refresh token is stored; the
-- plaintext token exists solely inside the httpOnly cookie.
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Unique: token-hash lookup doubles as replay detection once rotation
-- (a later slice) deletes the old row on refresh.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);

-- Plain index: multi-session listing and future bulk revocation by user.
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
