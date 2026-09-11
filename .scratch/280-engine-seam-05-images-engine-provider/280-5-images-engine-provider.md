---
title: Images engine provider
parent: engine seam - hand each domain its podman engine at the connection seam
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - images
  - testing
  - refactoring
  - podman-integration
author: vibe-agent
---

## Parent

Engine seam epic (280-0-engine-seam-master) - read its "Design decisions"
section first. Mirror the volumes tracer (280-2).

## What to build

The images service stops constructing the tunnel image engine inline and
depends on an images-owned engine provider. Pull progress - the reason this
domain exists as a streaming endpoint - becomes behaviour-testable through the
fake engine.

The images engine interface, in domain vocabulary, exact caller-width, with no
context parameter:

- List, Remove, Pull, Prune

The provider is the same single closure method the tracer establishes. Note
the images domain is the only one using the fork's ImageEngine (with its
unused FarmNode field left zero-value, as today).

## Acceptance criteria

- [ ] No inline engine construction remains in the images service
- [ ] The adapter file is the only images file importing the fork's tunnel
      packages
- [ ] Behaviour tests through the fake engine cover pull progress events and
      failure mid-pull, remove with and without force, prune reporting - not
      just invocation shape
- [ ] `go test ./...` green; integration tests unchanged

## Blocked by

280-2-volumes-engine-provider

## Technical notes

- Fake engines construct podman entities (ImageSummary, ImagePullReport,
  PruneReport) for now, with small fixture helpers; the entity-to-DTO epic
  deletes those fixtures later. Keep the slices orthogonal.
