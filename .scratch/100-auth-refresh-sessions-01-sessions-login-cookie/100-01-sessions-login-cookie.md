---
title: Sessions storage + login issues httpOnly refresh cookie
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

The persistence foundation of refresh sessions, plus the first user-visible piece: logging in now creates a server-side session and sets an httpOnly refresh cookie.

A new SQL migration adds a `sessions` table (id, user id with foreign key to users, token hash, created at, expires at) with a unique index on the token hash and an index on the user id. A session model and repository follow the existing repository pattern (interface beside the implementation, mock in the shared test utilities): create, look up by token hash, delete by token hash, and delete expired sessions for a user.

Refresh tokens are opaque 256-bit random values, base64url-encoded. Only their SHA-256 hex hash is ever persisted; the plaintext exists solely inside the cookie. Session TTL comes from a new config setting (`SESSION_TTL_DAYS`, default 30), wired through the app, the integration harness config, and the unit-test config helper.

The service's login method (from the prefactor) additionally creates a session row and returns the refresh token alongside the access token; the handler sets it as a cookie with `httpOnly`, `Secure`, `SameSite=Strict`, and `Path=/api/auth`. As lazy cleanup, login also purges that user's expired session rows — no background sweeper.

The refresh endpoint does not exist yet (next slice); this slice is demoable as: log in, observe the `Set-Cookie` header, and find only a hashed row in the database.

## Acceptance criteria

- [x] Migration adds the `sessions` table with the columns and indexes above, applied idempotently by the existing migration runner
- [x] Session repository implements create, get-by-token-hash, delete-by-token-hash, and delete-expired-for-user, with a mock added to the shared test utilities
- [x] `SESSION_TTL_DAYS` config setting (default 30) is loaded, validated, and set in the integration harness and unit-test config helpers
- [x] Generated refresh tokens are 256-bit random values in base64url; the database stores only their SHA-256 hex hash
- [x] Login creates a new session row per login (multi-session: no per-user limit) and purges that user's expired rows
- [x] Login response sets a cookie with `httpOnly`, `Secure`, `SameSite=Strict`, `Path=/api/auth`
- [x] The login response body is unchanged (still the access JWT)
- [x] Service tests: login inserts a session row whose stored value does not equal the plaintext token; expiry uses the configured TTL
- [x] Handler tests: login sets the cookie with all four required attributes
- [x] Integration test: login via the real server sets the cookie, and the sessions table contains a hashed row for that user
- [x] `go build ./...`, `go vet ./...`, `go test ./...` pass, integration tests pass

## Blocked by

- 100-00-prefactor
