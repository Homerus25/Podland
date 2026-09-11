---
title: Delete the test-only service interfaces and mock twins
parent: none
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - epic
  - backend
  - testing
  - refactoring
author: vibe-agent
---

## What to build

After the engine seam epic (280) lands, handler tests can drive the real
services over fake engines. The per-domain `ServiceInterface` types - whose
only referents are the handler field, a `var _` compile check, and a
hand-written mock twin - are then test scaffolding with no production reason
to exist, and roughly 950 lines of interface-plus-mock maintenance go with
them.

This epic deletes those pairs domain by domain: handlers take the concrete
service, handler tests run the real service against the fake engine, and the
backend-conventions line about handler contract tests against a testify
`Mock*Service` retires in favor of the engine-seam testing shape.

Not touched: the engine provider interfaces and their fakes (production
seams), `connections.Manager` and its mock (a production seam), the
repository mocks, and the sshexec mock.

## Vertical Slices

### 1. Small domains - volumes, secrets, images, containers
**Issue**: 290-1-small-domains
- Delete the ServiceInterface + Mock*Service pair per domain; handlers take
  the concrete service; handler tests drive the real service over the fake
  engine
- Retire the Mock*Service conventions line for these domains

**Status**: ready-for-agent
**Blocked by**: 280-2-volumes-engine-provider, 280-3-secrets-engine-provider,
280-4-containers-engine-provider, 280-5-images-engine-provider

### 2. Quadlets - the wide one
**Issue**: 290-2-quadlets
- Same deletion for the widest pair (13-method interface, ~105-line mock, the
  largest handler test file in the tree); final conventions sweep retiring
  the Mock*Service ruling repo-wide

**Status**: ready-for-agent
**Blocked by**: 280-6-quadlets-engine-provider

## Acceptance criteria

- [ ] No domain-local ServiceInterface or Mock*Service twin remains
- [ ] Handler tests drive real services over fake engines and assert domain
      routing and decoding - they no longer re-pin envelope behaviour that
      web.Handle owns and pins once
- [ ] backend-conventions.md describes the engine-seam testing shape as the
      only handler-test pattern
- [ ] `go test ./...` green throughout

## Blocked by

280-engine-seam (all six slices, via the slice-level edges above)
