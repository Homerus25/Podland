---
title: Prefactor — move login flow into AuthService
parent: 100-auth-refresh-sessions
date: 2026-08-18
status: open
labels:
  - ready-for-agent
  - backend
  - auth
  - refactor
author: vibe-agent
---

## Parent

100-auth-refresh-sessions — Revocable refresh sessions for authentication

## What to build

A pure prefactor with no behavior change: move the login business logic out of the auth handler and into the auth service. Today the handler performs the user lookup, the bcrypt password comparison, and the token generation itself, while the `AuthService` holds an injected user repository it never uses. Session creation (next slice) has to hook into login, and login logic in a handler would force session persistence into the wrong layer.

The handler keeps only HTTP concerns: request decoding, validation, and error mapping. The service gains a login method that performs user lookup, password verification, and access-token generation, returning the token and the authenticated user.

All externally observable behavior (routes, request/response shapes, status codes, error envelopes, timing of failures) must be identical before and after.

## Acceptance criteria

- [ ] `AuthService` exposes a login method that takes credentials, performs user lookup via its user repository, verifies the password, and returns a fresh access token plus the user
- [ ] The injected-but-unused user repository field on `AuthService` is now actually used for login
- [ ] `AuthHandler.Login` is reduced to DTO parsing, delegation to the service, and error mapping via the structured error responder
- [ ] No route, request/response shape, status code, or error code changes
- [ ] Login logic tests move to (or are added to) the service test suite, covering: unknown username, wrong password, and success
- [ ] Handler tests still cover missing/invalid request bodies and delegate happy path
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass, and integration auth tests pass unchanged

## Blocked by

None - can start immediately
