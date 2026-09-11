---
title: Engine seam - hand each domain its podman engine at the connection seam
parent: none
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - epic
  - backend
  - testing
  - refactoring
  - podman-integration
author: vibe-agent
---

## What to build

Deepen the connection seam (the podman prologue) so that every machine-scoped
podman call receives a domain-owned engine instead of a bare connection
context. Today 23 call sites construct the podman engine inline inside their
`WithConnection` closures, which leaves the engine unseamed: the services
behind it cannot be tested through their own interfaces, and the fork's tunnel
methods silently discard the context they are handed (the engine's `ClientCtx`
is the only context that ever reaches podman).

After this epic, each domain declares a narrow engine interface and a
one-method provider; a tiny production adapter per domain wraps
`connections.Manager.WithConnection` and constructs the tunnel engine (or
package-function wrappers) inside it. A per-domain fake engine makes every
service's real behaviour unit-testable: quadlet Update's remove+install
composition, Delete's precondition ordering, and GetLogs' name resolution
become behaviour tests instead of "an error happened" invocation-shape tests.

This epic also fixes, first, a live connection-lifetime bug the design review
surfaced: the cached connection context is parented to the cold-dialing
request's context, which net/http cancels the moment that request's handler
returns.

## Vertical Slices

### 1. Detach cached connection contexts from the dialing request
**Issue**: 280-1-detach-cached-connection-contexts
- Dial still honors the caller's context, but the stored connection context is
  detached (`context.WithoutCancel`) so its lifetime equals the connection's
  lifetime
- Regression test pins "connection context outlives the dialing request"
- Live two-request diagnostic before/after; the fix lands regardless of the
  repro's outcome

**Status**: ready-for-agent
**Blocked by**: None

### 2. Volumes engine provider (tracer)
**Issue**: 280-2-volumes-engine-provider
- Volumes becomes the reference domain: engine interface, provider, production
  adapter, fake engine, behaviour tests
- backend-conventions.md connection-seam ruling and CONTEXT.md gain the new
  reference shape

**Status**: ready-for-agent
**Blocked by**: 280-1-detach-cached-connection-contexts

### 3. Secrets engine provider
**Issue**: 280-3-secrets-engine-provider
- Pure package-function call style behind the same provider shape

**Status**: ready-for-agent
**Blocked by**: 280-2-volumes-engine-provider

### 4. Containers engine provider
**Issue**: 280-4-containers-engine-provider
- 7-method engine incl. streaming Logs; dead MockContainerEngine deleted

**Status**: ready-for-agent
**Blocked by**: 280-2-volumes-engine-provider

### 5. Images engine provider
**Issue**: 280-5-images-engine-provider
- 4-method engine; pull progress becomes behaviour-testable

**Status**: ready-for-agent
**Blocked by**: 280-2-volumes-engine-provider

### 6. Quadlets engine provider (payoff)
**Issue**: 280-6-quadlets-engine-provider
- 7-method engine incl. Install; Update/Delete/GetLogs become behaviour tests;
  resolveForLogs context wart disappears

**Status**: ready-for-agent
**Blocked by**: 280-2-volumes-engine-provider

Slices 3-6 are parallel-safe once the tracer lands.

## Design decisions (from the grilling session - do not re-litigate)

- **Provider shape**: one closure method per domain,
  `WithEngine(ctx, userID, machineID, op string, fn func(e <Domain>Engine) error) error`,
  delegating to `connections.Manager.WithConnection` with the op tag passed
  through. No per-operation provider methods.
- **Engine interfaces are exact caller-width, in domain vocabulary, with no
  context parameter.** The connection context lives only inside the production
  adapter (it alone feeds the package functions like quadlet Install and
  volume Export/Import). Services keep their own request context for selects
  and disconnect detection; the streaming seam's disconnect handling is
  unchanged. The dual-context trap is deleted, not closed.
- **Method sets** (the whole inventory, nothing speculative):
  - volumes: List, Inspect, Create, Remove, Prune, Export(writer), Import(reader)
  - secrets: List, Inspect, Create, Remove
  - containers: List, Create, Inspect, Start, Stop, Remove, Logs
  - images: List, Remove, Pull, Prune
  - quadlets: List, Print, Remove, Install, Logs, InspectContainer, InspectPod
- **Adapter placement**: one adapter file inside each domain package; it is the
  only file in the domain importing the fork's tunnel/bindings packages.
- **Fakes**: per domain, function-field style, colocated in the domain's
  tests; graduate to testutils only when a third domain needs the same one
  (the repo's third-copy threshold). Acquire failure is simulated by the fake
  provider returning ErrConnectionFailed; the Manager's warn-and-sentinel
  behaviour stays in Manager.WithConnection, untouched.
- **Behaviour tests pin current semantics.** Quadlet Update's non-atomicity
  (remove succeeds, install fails, quadlet gone) is pinned with an explicit
  test name/comment; making Update atomic is a future decision.
- **Orthogonality**: services keep returning podman entities until the
  entity-to-DTO epic; fake engines construct entity fixtures for now, and that
  epic deletes them later.
- **Out of scope**: sshexec (already a real seam), ProbeMachine and
  TestConnection (the Manager's own entry points), the machines domain, the
  bufferedLogs endpoint-envelope migration.

## Acceptance criteria

- [ ] No domain service constructs a podman engine inline; every machine-scoped
      podman call goes through its domain's engine provider
- [ ] Each domain's service tests exercise real behaviour through its fake
      engine, including the acquire-failure (502) path
- [ ] The fork's tunnel/bindings imports are confined to per-domain adapter
      files
- [ ] backend-conventions.md and CONTEXT.md describe the engine handover under
      the existing connection-seam concept (no new term)
- [ ] `go test ./...` green throughout; integration tests unchanged

## Blocked by

None - can start immediately with slice 1
