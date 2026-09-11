---
title: "User roles prefactor: role column, initdb bootstrap admin, status exposure"
date: 2026-08-17
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - auth
author: vibe-agent
---

## What to build

Introduce a role concept for users so that authorization decisions (starting with the user-creation endpoint) can distinguish admins from regular users. A migration adds a `role` column to the `users` table, defaulting to `user` so existing rows keep working unchanged. `cmd/initdb` — the bootstrap tool — creates its user with role `admin`. The role is readable end-to-end: a logged-in user can ask `GET /api/auth/status` and see their own role.

Out of scope: any endpoint that *enforces* the role (that is the next slice), changing machine-level permissions, and any UI.

## Acceptance criteria

- [ ] Migration adds `role` to `users` as `TEXT NOT NULL DEFAULT 'user'`; existing users end up with role `user` after migration runs
- [ ] Role values are constrained to `user` and `admin` (DB-level check or application-level validation)
- [ ] `cmd/initdb` creates the bootstrap user with role `admin`; re-running it against an existing admin remains idempotent
- [ ] The user model and repository read and persist the role
- [ ] `GET /api/auth/status` response includes the authenticated user's `role`
- [ ] Unit tests cover the repository role round-trip and the migration default; an integration test (080 harness) asserts an admin-role user sees `admin` in the status response while a default user sees `user`
- [ ] Auth API docs updated to document the `role` field on the status response

## Blocked by

None - can start immediately.

## Technical notes

- `username` is already `UNIQUE NOT NULL` in the initial schema; no change needed there.
- Whether the role travels as a JWT claim or is looked up per request is an implementation choice. If it goes into the token, note that role changes only take effect after token expiry (default 60 min) — acceptable for this app.
