-- Revoked access tokens: the jti denylist behind logout's per-device
-- access-token revocation (issue 441). One row per revoked access JWT:
-- its jti and the token's own exp — the row is dead weight once that
-- expiry passes, so login purges expired rows lazily (there is no
-- background sweeper).
CREATE TABLE IF NOT EXISTS revoked_access_tokens (
    jti TEXT PRIMARY KEY,
    expires_at TIMESTAMP NOT NULL
);

-- Plain index: the lazy expiry purge scans on expires_at.
CREATE INDEX IF NOT EXISTS idx_revoked_access_tokens_expires_at ON revoked_access_tokens(expires_at);

-- Per-user access-token revocation cutoff (issue 441): a password change
-- or admin password reset stamps now here, and every access JWT of that
-- user minted before the stamp fails validation. NULL = never revoked.
ALTER TABLE users ADD COLUMN access_tokens_revoked_at TIMESTAMP;
