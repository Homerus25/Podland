---
title: "Gap #2: verify password in auth.Login"
parent: 080-integration-tests
date: 2026-08-11
status: ready-for-agent
labels:
  - ready-for-agent
  - bug
  - backend
  - auth
  - security
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

Implement actual password verification in `POST /api/auth/login`. Today the handler fetches the user and issues a JWT **without checking the password** (the `// TODO: Verify password hash` in `auth.Login`). Use `bcrypt.CompareHashAndPassword` against the stored `PasswordHash`; return `INVALID_CREDENTIALS` (`401`) on mismatch or unknown user. This is a prod-code security fix, not test authoring.

## Acceptance criteria

- [ ] `auth.Login` compares the supplied password with the stored hash via `bcrypt.CompareHashAndPassword`
- [ ] wrong password → `401 INVALID_CREDENTIALS`; correct password still returns `200` + token
- [ ] unknown username → `401 INVALID_CREDENTIALS` (no user-enumeration leak)
- [ ] unit tests cover match / mismatch / unknown-user
- [ ] an integration test (extending the 080-01 harness auth) asserts wrong-password → `401`

## Blocked by

None.

## Technical notes

- `cmd/initdb` already bcrypt-hashes passwords, so existing seeded users keep working once verification is enabled — no migration needed.
- This unblocks a meaningful "wrong password → 401" integration assertion; until it lands, the 080-01 harness login is intentionally lenient.
