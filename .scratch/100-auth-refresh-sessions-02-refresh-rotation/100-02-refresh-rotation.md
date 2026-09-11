---
title: Refresh endpoint with session rotation
parent: 100-auth-refresh-sessions
date: 2026-08-18
status: open
labels:
  - ready-for-agent
  - backend
  - auth
  - feature
author: vibe-agent
---

## Parent

100-auth-refresh-sessions — Revocable refresh sessions for authentication

## What to build

The endpoint that makes the refresh cookie useful: a new, unauthenticated `POST /api/auth/refresh` that exchanges a valid refresh cookie for a fresh access JWT and a rotated refresh cookie.

The service owns the flow: read the cookie, hash it, look up the session by token hash. On success, rotate atomically — delete the old row, insert a new one with a fresh token and a renewed (sliding) expiry — and issue a new access JWT; the handler sets the new cookie with the same attributes as login (`httpOnly`, `Secure`, `SameSite=Strict`, `Path=/api/auth`). Rotation is the only refresh path; there is deliberately no non-rotating variant, so a replayed (already-rotated) token simply fails its lookup.

Failures return 401 with the structured error envelope and two new auth error codes: `AUTH_REFRESH_INVALID` for a missing cookie or unknown token hash (replay or revoked), and `AUTH_SESSION_EXPIRED` for a found-but-expired row, which is deleted on encounter as lazy cleanup. Two concurrent refreshes with the same cookie are safe: exactly one wins the rotation, the loser gets a clean 401, and the data stays consistent.

The integration harness gains a cookie-jar HTTP client helper (standard library cookie jar around the existing transport) — this suite is its first consumer. Demoable: log in, refresh with the cookie (new token + new cookie), replay the old cookie → 401.

## Acceptance criteria

- [ ] `POST /api/auth/refresh` is registered as a public (unauthenticated) route
- [ ] Valid cookie → 200 with a new access JWT in the body and a new refresh cookie with the four required attributes
- [ ] Rotation deletes the old session row and inserts a new one with a fresh token and renewed sliding expiry
- [ ] Missing cookie or unknown token hash → 401 with error code `AUTH_REFRESH_INVALID`
- [ ] Expired session → 401 with error code `AUTH_SESSION_EXPIRED`, and the expired row is deleted
- [ ] Replaying a rotated token fails with `AUTH_REFRESH_INVALID`
- [ ] Concurrent refreshes with the same cookie: exactly one succeeds, the other fails cleanly, no data corruption
- [ ] Service tests: valid rotation (new hash differs, old hash gone, expiry renewed), replay, expired, unknown-hash cases
- [ ] Handler tests: cookie parsing, response envelope shape and codes, new cookie attributes
- [ ] Integration harness has a cookie-jar client helper reusable by later suites
- [ ] Integration test: login → refresh succeeds → old-cookie replay fails → second device's independent session still refreshes fine
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass, integration tests pass

## Blocked by

- 100-01-sessions-login-cookie
