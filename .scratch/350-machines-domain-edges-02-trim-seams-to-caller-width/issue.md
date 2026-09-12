---
title: Trim the machines domain's seams to caller width
parent: 350-machines-domain-edges
date: 2026-09-12
status: open
labels:
  - ready-for-agent
  - backend
  - machines
  - refactoring
author: vibe-agent
---

## Parent

`.scratch/350-machines-domain-edges/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

Every seam the machines service declares becomes exactly as wide as its
callers. The repository interface drops `ListByOwner` — the service never
calls it; the only exercisers are tests of the mock itself, which are tests
of dead behavior and go with it. The encryption seam shrinks from a four-
method interface to a single unexported one-method interface
(`sshKeyEncryptor`, method `EncryptSSHKey`), mirroring the unexported
`sshInvalidator` pattern the domain already uses; the shared testutils mock
shrinks to the same width, its error-injecting constructor reduced to one
SSH-key error.

connections and sshexec are untouched: they depend on the real encryption
service directly, not on this seam. No behavior change anywhere — the seams
just stop declaring what nobody calls.

## Acceptance criteria

- [ ] Repository interface is exactly the service's call width: six methods (Create, GetByID, Update, Delete, ListAccessible, NameExistsForUser)
- [ ] Mock-only tests that exercised `ListByOwner` (the mock's self-tests in testutils and the service-test subtests calling it on the mock) are deleted together with the mock's `ListByOwner`
- [ ] The exported `EncryptionServiceInterface` is gone; the seams file declares one unexported one-method encryption interface consumed by the service
- [ ] `testutils.MockEncryptionService` implements exactly one method; its error constructor takes a single SSH-key error; the custom-functions constructor exposes only what the seam needs
- [ ] The machines service constructor's encryption parameter is the narrowed seam type; existing call sites (real service in app.go, mock in tests) pass their concretes unchanged in substance
- [ ] connections, sshexec, and the encryption package itself are not modified
- [ ] `go vet ./...` and `go test ./...` green in the backend module

## Blocked by

- `350-machines-domain-edges-01-delete-dead-twin` (the interface being trimmed lives in the seams file this slice's blocker creates)

## Technical notes

- The service's encryption field and constructor parameter change type; that is the only signature movement. Go callers outside the package keep passing concrete values, so no import churn.
