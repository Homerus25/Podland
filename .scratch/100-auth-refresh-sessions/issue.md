---
title: Revocable refresh sessions for authentication
parent: none
date: 2026-08-18
status: open
labels:
  - ready-for-agent
  - backend
  - auth
  - feature
author: vibe-agent
---

## Problem Statement

As a user of the Podland web app, I have to choose between two bad options today: either the frontend stores my long-lived access JWT in localStorage (where any XSS bug can steal it), or it keeps the token in memory only and forces me to log in again on every page refresh or tab reopen. The backend issues a single Bearer JWT with no way to renew it, and logout is a client-side no-op that leaves nothing to revoke. There is no way to stay conveniently logged in on multiple devices without long-lived credentials floating around in the browser.

## Solution

Introduce server-side, revocable refresh sessions. Logging in returns the existing short-lived access JWT and additionally sets an httpOnly cookie containing an opaque refresh token. The refresh token is stored only as a SHA-256 hash in a new sessions table, one row per login, so a user can stay logged in on multiple devices. A new refresh endpoint exchanges a valid cookie for a fresh access JWT and rotates the cookie. Logout becomes real: it revokes the session row and clears the cookie. Because the cookie is httpOnly, JavaScript (including XSS payloads) can never read it, and because tokens rotate on every refresh, a stolen cookie has a short useful life.

## User Stories

1. As a user, I want to stay logged in across page refreshes and browser restarts, so that I don't have to re-enter my password every time I open the dashboard.
2. As a user, I want to be logged in on my laptop and my phone at the same time, so that I can check my machines from anywhere.
3. As a user, I want logging out on one device to only end that device's session, so that my other devices keep working.
4. As a user, I want the refresh token to be unreadable to JavaScript, so that an XSS bug in the frontend cannot steal a long-lived credential.
5. As a user, I want a stolen refresh token to become useless as soon as I use my session again, so that attackers get a very short window (rotation on every refresh).
6. As a security-conscious user, I want refresh tokens stored only as hashes in the database, so that a database leak does not yield usable credentials.
7. As a user, I want an expired session to be treated the same as a logged-out session, so that behavior is predictable (silent re-login prompt, no errors).
8. As a user, I want a replayed (already-rotated) refresh token to be rejected, so that token theft is detectable and useless.
9. As a user with a slow device, I want concurrent refresh calls to be safe, so that parallel tabs don't invalidate each other's session.
10. As an operator, I want refresh attempts rate-limited like login attempts, so that cookie brute-forcing is impractical.
11. As a developer, I want all existing Bearer-token API behavior untouched, so that streaming endpoints and clients keep working without changes.
12. As a developer, I want expired sessions purged without a background job, so that the table stays clean with zero new infrastructure.

## Implementation Decisions

- New `sessions` table via the existing SQL migration mechanism: `id`, `user_id` (FK to users), `token_hash` (SHA-256 hex), `created_at`, `expires_at`; unique index on `token_hash` for lookup, index on `user_id` for multi-session and future bulk revocation.
- Refresh tokens are opaque 256-bit random values (base64url). Only the hash is persisted; the plaintext exists only inside the cookie.
- Multi-session model: every login inserts a new row. No per-user session limit initially.
- `POST /api/auth/login` keeps its current response body (access JWT) and additionally sets the refresh cookie: `httpOnly`, `Secure`, `SameSite=Strict`, `Path=/api/auth`.
- New endpoint `POST /api/auth/refresh`: reads the cookie, hashes it, looks up the session. On success it rotates: delete the old row, insert a new one with a fresh token and fresh expiry, set the new cookie, return a new access JWT. On failure respond 401 with the structured error envelope.
- Rotation is the only refresh path; there is no non-rotating refresh. Reuse of an already-rotated token hash simply fails lookup (the row is gone).
- `POST /api/auth/logout` becomes real: if a valid session cookie is present, revoke that row; always clear the cookie. Remains idempotent and succeeds without a cookie.
- Refresh session TTL: 30 days, renewed at every rotation (sliding). Access JWT lifetime stays configurable via the existing `JWT_EXPIRATION_MINUTES` setting (default 60) and can be lowered later without frontend changes.
- Lazy cleanup: expired rows for a user are deleted on that user's login and on any refresh lookup miss; no background sweeper.
- `GET /api/auth/status` and every other endpoint are unchanged: they continue to accept only the Bearer access token. Image pull / container log NDJSON streaming and the future SSE endpoint are unaffected.
- `/api/auth/refresh` is added to the existing auth rate-limit group alongside login.
- New error codes in the structured envelope: `AUTH_REFRESH_INVALID` and `AUTH_SESSION_EXPIRED` (both 401).
- Same-origin deployment behind Caddy means no CORS-with-credentials work is needed.

## Testing Decisions

- Good tests assert external behavior only: what the HTTP layer returns, which cookies are set with which attributes, and what subsequent requests with those cookies do.
- Service-level unit tests extend the existing auth service suite (prior art: `011-auth-service-tests`): login inserts a hashed session row; refresh with a valid token rotates it; the old token fails on replay; an expired session fails; logout revokes exactly its own row; hash lookup never matches plaintext.
- Handler-level tests cover cookie attributes (httpOnly, Secure, SameSite, Path) on login/refresh/logout, and the 401 error envelope shape for invalid/expired refresh.
- Integration tests through the real HTTP server with a cookie jar (prior art: the `080-integration-tests` harness, slices 05/06): full cycle login → status → refresh → API call with new access token → logout → refresh now fails; second device still refreshes fine after first device logs out.
- Migration test: the sessions table is created by the existing migration runner and is idempotent.

## Out of Scope

- Frontend changes consuming this flow (that is PRD `110-frontend-skeleton`).
- Password change and revocation-on-password-change (no password change endpoint exists yet).
- "Revoke all sessions" / admin session-management API or UI.
- Session metadata such as device names, IPs, or a last-active sessions list.
- Reducing the default access-token lifetime (config-only change, can happen anytime).

## Further Notes

- Sequencing: this is the first PRD of the frontend arc; `110-frontend-skeleton` is blocked by it.
- Rotation-on-refresh plus hashed storage was chosen deliberately over non-rotating cookies (stolen cookie valid for full TTL) and over non-DB revocable designs (logout stays a no-op).
- Access token stays Bearer-header-only so streaming endpoints (which cannot trivially re-send cookies through `fetch` in all flows) and the future SSE channel need no special casing.
