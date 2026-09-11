---
title: Rate limiting for auth endpoints (login + refresh)
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

Brute-force protection for the cookie-based auth surface. Note a correction to the parent PRD's assumption: auth routes are **not** currently rate-limited at all — the existing rate-limit middleware only wraps JWT-protected routes and keys on the authenticated user ID from context, explicitly skipping unauthenticated requests. Login and refresh are unauthenticated, so they need their own limiter with a different key.

Build a small in-memory limiter (reuse the existing token-bucket implementation) keyed by client IP for the login and refresh endpoints, registered as a middleware group around those two routes. Exceeding the limit returns the standard 429 response with the structured error envelope and rate-limit response headers, consistent with the existing middleware's behavior. The default limit should be tight (brute-force hostile) and configurable via a new environment setting alongside the existing rate-limit config; the integration harness raises it like it does the existing one.

The existing per-user API rate limiting must remain untouched.

## Acceptance criteria

- [ ] Login and refresh share an IP-keyed rate-limit group; logout is not limited
- [ ] Requests over the limit receive 429 with the structured error envelope and rate-limit headers, consistent with existing middleware behavior
- [ ] The limit is configurable via a new environment setting with a secure default, documented in the backend env example file
- [ ] The limiter keys by client IP, not by authenticated user ID, since these routes are unauthenticated
- [ ] Integration harness sets a high limit so existing suites are unaffected
- [ ] Middleware unit tests: under-limit passes, over-limit 429s, keys are per-IP
- [ ] Integration test: with a low configured limit, repeated login or refresh attempts eventually receive 429
- [ ] Existing per-user API rate limiting behavior is unchanged
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass, integration tests pass

## Blocked by

- 100-02-refresh-rotation
