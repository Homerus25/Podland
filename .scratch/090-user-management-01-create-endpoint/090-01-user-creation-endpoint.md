---
title: "Admin-only POST /api/users endpoint for creating a new user"
date: 2026-08-17
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - auth
author: vibe-agent
---

## Parent

090-user-management — User creation with roles (role prefactor is 090-00)

## What to build

An authenticated, admin-only endpoint that creates a new user. `POST /api/users` accepts `username` and `password`, bcrypt-hashes the password, and returns `201` with the created user (id, username, role, createdAt — never the password hash). Created users get role `user` and can immediately log in via `POST /api/auth/login`. Unauthenticated calls get `401`; authenticated non-admins get `403`; duplicate usernames get `409`; missing/empty fields get `400`. The creation logic (exists-check, hash, insert) is extracted so the endpoint and `cmd/initdb` share one path rather than duplicating it.

## Acceptance criteria

- [ ] `POST /api/users` is registered behind the JWT middleware; caller without a valid token → `401`
- [ ] Caller with role `user` → `403`; caller with role `admin` → creation proceeds
- [ ] Happy path returns `201` with `{id, username, role, createdAt}` and `role` is `user`; the password is stored bcrypt-hashed, never echoed
- [ ] Duplicate username → `409`; missing or empty username/password → `400`; a malformed body → `400`
- [ ] A user created through the endpoint can successfully log in with the given credentials (integration test)
- [ ] User-creation logic is shared between the endpoint and `cmd/initdb` (single service/repository path for hash + exists-check + insert); the UNIQUE username constraint acts as a backstop for the concurrent-duplicate race and surfaces as `409`
- [ ] Unit tests cover the handler (201/400/401/403/409) and the creation service; integration tests via the 080 harness cover admin-creates-user → new user logs in, and non-admin → 403
- [ ] API docs updated (auth/users API documentation) and a `resonance/podland` request file for the new endpoint is added

## Blocked by

- 090-user-management-00-role-prefactor (`090-00-user-role-prefactor.md`) — the admin gate requires the role concept to exist

## Technical notes

- Deliberately no `role` field in the request payload: admins create regular users only, so privilege escalation requires direct DB access. If admin-creation is ever needed, that deserves its own discussion.
