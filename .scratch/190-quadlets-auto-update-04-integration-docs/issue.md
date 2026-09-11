---
title: "Auto-update integration test and API documentation"
parent: 190-quadlets-auto-update
date: 2026-08-30
status: ready-for-agent
labels:
  - ready-for-agent
  - quadlets
  - backend
  - feature
  - integration-tests
  - documentation
author: vibe-agent
---

## Parent

`.scratch/190-quadlets-auto-update/issue.md` — Quadlet auto-update with live streaming output

## What to build

Close out the auto-update feature with end-to-end coverage and the public contract.

**Integration test** against the existing podman harness, in dry-run mode only (real update runs hit live registries and are too flaky for the harness — explicitly out of scope). The harness needs a quadlet fixture that declares an auto-update policy (`AutoUpdate=registry` in the `.container` file), since policy-less quadlets are invisible to `podman auto-update`. Verify the endpoint contract end-to-end: unauthenticated requests rejected; read-only access rejected at write level; write access streams the NDJSON event sequence (log events, then exactly one terminal complete event carrying the report); and the dry-run performs no mutation (the fixture's container is untouched afterwards).

**API documentation**: the quadlet API documentation gains the endpoint's contract — route, permission level, the `dryRun` query parameter, the NDJSON event schema with concrete examples for log/complete/error events, the per-container report entry shape, and the cancellation semantics (disconnect aborts the run) plus the extended response deadline behavior.

## Acceptance criteria

- [ ] Harness fixture: a quadlet declaring `AutoUpdate=registry` (policy-less quadlets alone would make the endpoint a no-op)
- [ ] Integration test: unauthenticated request rejected
- [ ] Integration test: read-only access rejected (403), write access accepted
- [ ] Integration test: dry-run stream returns log events followed by exactly one terminal complete event with a parseable report
- [ ] Integration test: dry-run leaves the fixture's container unmutated
- [ ] Quadlet API documentation documents route, permissions, `dryRun` parameter, event schema with examples, report shape, and cancellation semantics
- [ ] Existing integration suite still passes; `go build ./...` and `go vet ./...` clean

## Blocked by

- 190-quadlets-auto-update-02-backend-auto-update-endpoint (the endpoint under test)
