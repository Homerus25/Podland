---
title: Delete the dead mock twins in testutils
parent: 440-dead-surface
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - backend
  - testing
  - refactoring
author: vibe-agent
---

## Parent

`.scratch/440-dead-surface/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

testutils stops maintaining two mock types that mock nothing real.
`MockPodmanConnectionService` and `MockPodmanConnection` satisfy no
production interface — every method traffics in `any` — and their only
caller in the whole repo is testutils' own self-test, which asserts
that the mocks return what the mocks were written to return. After this
slice the package's mock surface is exactly the mocks that satisfy real
production seams, and the backend suite is green.

## Acceptance criteria

- [ ] `MockPodmanConnectionService` and `MockPodmanConnection` are gone from `internal/testutils`, together with their constructors (`NewMockPodmanConnectionService`, `NewMockPodmanConnectionServiceWithErrors`).
- [ ] The self-test `TestMockPodmanConnection` (both subtests) in `testutils_test.go` is deleted — it existed only to exercise the deleted types.
- [ ] A repo-wide search finds no reference to either type name outside git history.
- [ ] No import is removed from the package that other mocks still need (`sshexec` stays for `MockSSHExec`; `bindings` stays for `mockConnectionContext`).
- [ ] The surviving mocks' self-tests (`TestMockConnectionManager_*`, `MockSSHExec` pins, `MockEncryptionService` users) are untouched and green.
- [ ] `go build ./...`, `go vet ./...`, and `go test ./...` pass in `backend/`; touched files pass `gofmt`.

## Blocked by

- None (can start immediately)

## Technical notes

- Pure deletion, ~45 lines. Nothing production-side references these types, so no wiring or migration exists to do.
