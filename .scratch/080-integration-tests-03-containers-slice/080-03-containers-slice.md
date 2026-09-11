---
title: Containers vertical slice — create, start, stop, get, delete over real podman
parent: 080-integration-tests
date: 2026-08-11
status: ready-for-agent
labels:
  - ready-for-agent
  - testing
  - backend
  - integration-tests
  - podman-integration
  - containers
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

Integration tests covering the container lifecycle over the real podman machine, reusing the harness from 080-01. Ensure `busybox` is present (idempotent pull — cheap when cached), then create, start, stop, get, and delete a container. This proves the container-management request flow end-to-end through the SSH tunnel.

## Acceptance criteria

- [ ] test ensures `busybox` exists on the machine (idempotent pull) before creating containers
- [ ] `POST /api/machines/{id}/containers` creates a run-scoped-named container (e.g. `busybox sh`); `201` returned
- [ ] start → container reaches `running`; stop → exits; `GET .../containers/{containerId}` reflects the state transitions
- [ ] `DELETE .../containers/{containerId}` removes the container; a subsequent list excludes it
- [ ] all created containers use run-scoped names (`it-<runID>-…`); per-test `t.Cleanup` + the suite sweep delete them
- [ ] test passes under `make test-integration`

## Blocked by

- 080-01-harness-tracer-bullet (needs the harness, authenticated client, and run-scoped tag/cleanup helpers)

## Technical notes

- Use a short-lived command (e.g. `busybox sh -c "sleep 30"` or `top`) so start/stop transitions are observable without the container exiting immediately.
- Do not depend on test execution order within the package — each test ensures its own prerequisites.
