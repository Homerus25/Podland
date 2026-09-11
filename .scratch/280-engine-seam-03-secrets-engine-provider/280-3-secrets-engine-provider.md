---
title: Secrets engine provider
parent: engine seam - hand each domain its podman engine at the connection seam
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - secrets
  - testing
  - refactoring
  - podman-integration
author: vibe-agent
---

## Parent

Engine seam epic (280-0-engine-seam-master) - read its "Design decisions"
section first. Mirror the volumes tracer (280-2) exactly; this slice proves
the pattern for a domain with no tunnel engine at all.

## What to build

The secrets service stops calling the fork's package-level bindings functions
directly on the connection context and instead depends on a secrets-owned
engine provider. Secrets is the purest package-function domain - it never
constructs a tunnel engine - so this slice shows the adapter shape where every
method wraps a package function and the connection context stays inside the
adapter.

The secrets engine interface, in domain vocabulary, exact caller-width, with no
context parameter:

- List, Inspect, Create, Remove

The provider is the same single closure method the tracer establishes,
delegating to `connections.Manager.WithConnection`. A function-field fake
engine, colocated with the secrets tests, drives behaviour tests: list, inspect
(with the admin reveal gate staying service-side, per the recorded ruling),
create, and remove error paths, plus the acquire-failure path via the fake
provider returning ErrConnectionFailed.

## Acceptance criteria

- [ ] No direct fork bindings calls remain in the secrets service; all four go
      through the engine interface
- [ ] The adapter file is the only secrets file importing the fork's bindings
      packages
- [ ] Behaviour tests through the fake engine cover list, inspect (including
      the reveal gate), create, and remove - not just invocation shape
- [ ] `go test ./...` green; integration tests unchanged

## Blocked by

280-2-volumes-engine-provider

## Technical notes

- The reveal gate (admin-level query parameter checked inside the service)
      is a deliberate, recorded ruling - it moves nowhere.
- Fake engines construct podman entities for now; the entity-to-DTO epic
  deletes those fixtures later.
