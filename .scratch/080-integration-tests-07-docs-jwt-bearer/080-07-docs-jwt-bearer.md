---
title: "Gap #3: docs — JWT is Authorization: Bearer, not HTTP-only cookie"
parent: 080-integration-tests
date: 2026-08-11
status: ready-for-agent
labels:
  - ready-for-agent
  - docs
  - auth
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

Correct the auth documentation to match the implementation. `CONTEXT.md` §5 and `description_backend.md` §Authentication claim JWTs are stored in **HTTP-only cookies**; the backend actually issues the token in the login JSON body (`LoginResponse{Token}`) and the middleware reads `Authorization: Bearer` (`internal/middleware/auth.go`). Update the docs to describe Bearer-header transport. This is a doc-only fix.

## Acceptance criteria

- [ ] `CONTEXT.md` §5 describes `Authorization: Bearer` transport (no HTTP-only-cookie claim)
- [ ] `description_backend.md` Authentication section updated to match the Bearer implementation
- [ ] no "HTTP-only cookie" JWT claim remains anywhere in the docs
- [ ] note that password verification is tracked separately (080-06), so the docs don't overstate login security

## Blocked by

None.

## Technical notes

- Source of truth is the code: `internal/domain/auth/handler.go` (`LoginResponse{Token}`) and `internal/middleware/auth.go` (reads `Authorization: Bearer`).
- The login-doesn't-verify-password caveat is a separate fix (080-06); do not claim login is fully secure until that lands.
