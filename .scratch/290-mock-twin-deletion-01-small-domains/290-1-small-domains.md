---
title: Mock-twin deletion - volumes, secrets, images, containers
parent: delete the test-only service interfaces and mock twins
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - testing
  - refactoring
  - volumes
  - secrets
  - images
  - containers
author: vibe-agent
---

## Parent

Mock-twin deletion epic (290-0-mock-twin-deletion-master). Blocked by the
engine seam epic: each domain's deletion can only happen once that domain's
service runs under a fake engine (its engine provider slice).

## What to build

For volumes, secrets, images, and containers: delete the domain-local
`ServiceInterface` and its hand-written `Mock*Service` twin. Handlers take the
concrete service. Handler tests stop stubbing the service and instead drive
the real one over the domain's fake engine - which means they stop asserting
"the handler forwarded (userID, machineID, ...) and mapped the error", since
the endpoint envelope owns that behaviour and pins it once. What the handler
tests keep and newly gain: routing decisions, URL/query decoding, and the
service's real behaviour visible through the HTTP surface.

One domain at a time, each landing green: interface gone, mock gone, handler
field retyped, handler tests rewired, `var _` compile checks gone.

## Acceptance criteria

- [ ] No ServiceInterface or Mock*Service twin remains in volumes, secrets,
      images, or containers
- [ ] Handler tests in those domains run the real service over the fake
      engine; no test re-pins envelope behaviour (identity extraction, error
      mapping, 500 fallback) that web.Handle pins centrally
- [ ] The testify mock for the connection manager and the engine provider
      fakes are untouched
- [ ] backend-conventions.md's handler-contract-test line reflects the new
      shape for these four domains
- [ ] `go test ./...` green after each domain lands

## Blocked by

280-2-volumes-engine-provider, 280-3-secrets-engine-provider,
280-4-containers-engine-provider, 280-5-images-engine-provider

## Technical notes

- Order within this ticket: start with volumes (smallest mock, reference
  domain), then secrets and images, containers last (its handler tests have
  the most cases among the four).
- If a handler test's only value was envelope re-pinning, delete it rather
  than port it - that is the point of this ticket.
