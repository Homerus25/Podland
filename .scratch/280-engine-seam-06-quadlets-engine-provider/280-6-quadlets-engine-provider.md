---
title: Quadlets engine provider (payoff)
parent: engine seam - hand each domain its podman engine at the connection seam
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - quadlets
  - testing
  - refactoring
  - podman-integration
author: vibe-agent
---

## Parent

Engine seam epic (280-0-engine-seam-master) - read its "Design decisions"
section first. Mirror the volumes tracer (280-2). This slice is the epic's
payoff: the backend's largest module (the quadlets service) becomes testable
through its own interface.

## What to build

The quadlets service stops constructing the tunnel engine inline and depends
on a quadlets-owned engine provider. The quadlets engine interface, in domain
vocabulary, exact caller-width, with no context parameter:

- List, Print, Remove, Install, Logs, InspectContainer, InspectPod

Install wraps the fork's package-level quadlets Install function (the
connection context it needs lives inside the production adapter). Generate's
container and pod inspection go through InspectContainer/InspectPod.
Lifecycle (Start/Stop/Restart) resolves its target via the engine inside the
same provider closure as today, and keeps going through sshexec for systemctl -
sshexec is a real seam already and moves nowhere.

The behaviour tests this slice unlocks, replacing the invocation-shape tests
whose own comments say what they cannot exercise:

- **Update composition**: remove succeeds then install fails - the quadlet is
  gone. Pin this non-atomicity explicitly: the test name and comment say so,
  and making Update atomic stays a future decision. Do not fix it here.
- **Delete precondition ordering**: not-found, running-guard with and without
  force, the remove itself.
- **GetLogs resolution**: quadlet name to container name via List/Print, then
  the container log stream. The resolveForLogs helpers' `context.Background()`
  calls disappear with the context-less engine interface - today those calls
  work only because the fork's tunnel methods discard the context anyway.

## Acceptance criteria

- [ ] No inline engine construction remains in the quadlets service or
      lifecycle code
- [ ] The adapter file is the only quadlets file importing the fork's
      tunnel/bindings packages
- [ ] Update's remove+install composition, Delete's precondition ordering, and
      GetLogs resolution are behaviour tests against the fake engine
- [ ] The non-atomic Update semantics are pinned in an explicitly named,
      commented test
- [ ] The InvocationShape tests that these behaviour tests replace are
      deleted; tests for the pure helpers stay
- [ ] No `context.Background()` remains in the quadlets podman paths
- [ ] `go test ./...` green; integration tests unchanged

## Blocked by

280-2-volumes-engine-provider

## Technical notes

- Fake engines construct podman entities for now; the entity-to-DTO epic
  deletes those fixtures later. Keep the slices orthogonal.
- The quadlet running-refusal phrases and lifecycle stderr classification stay
  quadlet-local per the podmanerr ruling - they are vocabulary, not seam,
  questions.
