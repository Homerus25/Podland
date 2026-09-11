---
title: Volumes engine provider (tracer)
parent: engine seam - hand each domain its podman engine at the connection seam
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - volumes
  - testing
  - refactoring
  - podman-integration
author: vibe-agent
---

## Parent

Engine seam epic (280-0-engine-seam-master) - read its "Design decisions"
section first; this slice is the tracer that turns those decisions into the
reference implementation every other domain mirrors.

## What to build

The volumes service stops constructing the podman engine inline inside its
`WithConnection` closures and instead depends on a volumes-owned engine
provider. The volumes domain becomes the conventions' reference for the engine
handover, and its tests stop being invocation-shape tests: list, inspect,
create, delete, prune, export, and import behaviour is exercised through a
fake engine, including error paths and the acquire-failure (502 CONNECTION_FAILED)
path.

The volumes engine interface, in domain vocabulary, exact caller-width, with no
context parameter:

- List, Inspect, Create, Remove, Prune (wrap the fork's tunnel engine methods)
- Export(writer), Import(reader) (wrap the fork's package functions; the
  connection context they need lives inside the production adapter)

The provider is one closure method,
`WithEngine(ctx, userID, machineID, op string, fn func(e VolumeEngine) error) error`,
delegating to `connections.Manager.WithConnection` with the op tag passed
through. The production adapter lives in a single file in the volumes package
and is the only volumes file importing the fork's tunnel/bindings packages.

## Acceptance criteria

- [ ] No inline engine construction remains anywhere in the volumes service
- [ ] The fake engine (function-field style, colocated with the volumes tests)
      drives behaviour tests: create-then-inspect flow, delete error paths,
      prune report mapping, export/import - not just "an error happened"
- [ ] Acquire failure is tested via the fake provider returning
      ErrConnectionFailed; Manager.WithConnection's warn-and-sentinel behaviour
      is untouched
- [ ] The adapter file is the only volumes file importing the fork's
      tunnel/bindings packages
- [ ] backend-conventions.md's "Connection seam (the podman prologue)" ruling
      shows the new reference shape (volumes remains the reference); CONTEXT.md
      gains one sentence describing the engine handover under the existing
      connection-seam concept - no new term is coined
- [ ] `go test ./...` green; integration tests unchanged

## Blocked by

280-1-detach-cached-connection-contexts

## Technical notes

- Volumes exercises both call styles in one domain (tunnel engine methods plus
  the Export/Import package functions), which is why it is the tracer.
- The fork's tunnel methods discard the context argument they are handed and
  use the engine's ClientCtx internally - that is why the engine interface
  carries no context at all, and why passing outer contexts today is
  decorative.
- Fake engines construct podman entities for now; the entity-to-DTO epic
  deletes those fixtures later. Keep the slices orthogonal.
