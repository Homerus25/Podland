---
title: Containers engine provider
parent: engine seam - hand each domain its podman engine at the connection seam
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - containers
  - testing
  - refactoring
  - podman-integration
author: vibe-agent
---

## Parent

Engine seam epic (280-0-engine-seam-master) - read its "Design decisions"
section first. Mirror the volumes tracer (280-2).

## What to build

The containers service stops constructing the tunnel engine inline and depends
on a containers-owned engine provider. This is the domain with the widest
engine surface and the first streaming consumer, so its fake engine also
demonstrates channel semantics for logs.

The containers engine interface, in domain vocabulary, exact caller-width,
with no context parameter:

- List, Create, Inspect, Start, Stop, Remove, Logs

The provider is the same single closure method the tracer establishes. The
dead MockContainerEngine - a mock whose expectations can never fire because
the engine is built inline - is deleted as part of this slice.

Streaming logs keep their current context discipline: the engine hands back
the log channel; disconnect detection stays on the request context at the
streaming seam, unchanged.

## Acceptance criteria

- [ ] No inline engine construction remains in the containers service
- [ ] The adapter file is the only containers file importing the fork's
      tunnel packages
- [ ] The fake engine simulates log-channel semantics (blocking until closed)
      so GetLogs and GetLogsWithStream behaviour is testable: follow and
      non-follow, disconnect cancellation, terminal handling
- [ ] Lifecycle sequences become behaviour tests: start-then-inspect and
      stop-then-inspect report the post-action state
- [ ] The dead MockContainerEngine and its never-firing expectations are
      deleted
- [ ] `go test ./...` green; integration tests unchanged

## Blocked by

280-2-volumes-engine-provider

## Technical notes

- The fork's ContainerLogs tunnel method discards its context argument and
  uses the engine's ClientCtx internally; the log channel's lifetime is
  governed by the connection context, which now lives only inside the adapter.
- Fake engines construct podman entities (ListContainer,
  ContainerInspectReport, ...) for now, with small fixture helpers; the
  entity-to-DTO epic deletes those fixtures later. Keep the slices orthogonal.
