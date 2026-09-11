---
title: Mock-twin deletion - quadlets, and the final conventions sweep
parent: delete the test-only service interfaces and mock twins
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - testing
  - refactoring
  - quadlets
author: vibe-agent
---

## Parent

Mock-twin deletion epic (290-0-mock-twin-deletion-master). Blocked by the
quadlets engine provider slice, since this ticket rewires the largest handler
test file in the tree against the real quadlets service.

## What to build

Delete the quadlets `ServiceInterface` (13 methods) and its ~105-line
`Mock*Service` twin. The quadlets handler takes the concrete service; its
handler tests - the tree's largest set - drive the real service over the
quadlets fake engine. Where a test existed only to assert forwarding and
envelope mapping, delete it; where it encodes routing or decoding decisions
(auto-update, logs dual-mode, import), keep and port it.

This ticket also performs the final conventions sweep: backend-conventions.md
now names the engine-seam testing shape (real service over fake engine,
envelope behaviour pinned once centrally) as the only handler-test pattern,
with no Mock*Service sentence left anywhere.

## Acceptance criteria

- [ ] QuadletServiceInterface and MockQuadletService are gone; the quadlets
      handler takes the concrete service
- [ ] The quadlets handler tests run the real service over the fake engine
- [ ] No Mock*Service type remains anywhere in the tree (the connection
      manager mock, repository mocks, and sshexec mock are not Service mocks
      and stay)
- [ ] backend-conventions.md describes one handler-test pattern; the
      Mock*Service ruling is retired repo-wide
- [ ] `go test ./...` green

## Blocked by

280-6-quadlets-engine-provider

## Technical notes

- The handler test file is large; work in small green steps (one test group
  at a time) rather than a single wholesale rewrite.
- Deleting a forwarded-args assertion is the win, not a loss - the envelope
  and the service behaviour tests cover what it used to check indirectly.
