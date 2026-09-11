---
title: Logout revokes the refresh session
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

Make logout real. Today `POST /api/auth/logout` is a no-op. Now: if the request carries a valid session cookie, that session's row is revoked (deleted); in every case the refresh cookie is cleared by setting an expired cookie with the same attributes. Logout stays idempotent and returns success even with no cookie at all, and its response shape does not change.

Revocation is per-session: logging out on one device ends only that device's session, leaving other devices' sessions untouched.

Demoable as the full session lifecycle: log in on two devices, log out on device A, device A's refresh fails while device B's refresh still succeeds.

## Acceptance criteria

- [ ] Logout with a valid session cookie deletes that session row and clears the cookie (expired cookie, same name/path/attributes)
- [ ] Logout without a cookie still succeeds with the existing response shape (idempotent)
- [ ] Logout with an unknown cookie hash succeeds and clears the cookie (no error on already-revoked)
- [ ] Only the presented session is revoked; other sessions of the same user remain valid
- [ ] Service tests: revoke deletes exactly the target row; no-op cases don't error
- [ ] Handler tests: cookie-clearing response headers; unchanged response shape
- [ ] Integration test: device A and device B log in (separate cookie jars), A logs out, A's refresh returns 401 while B's refresh returns a new token
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass, integration tests pass

## Blocked by

- 100-02-refresh-rotation
