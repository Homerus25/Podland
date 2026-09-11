---
title: Images vertical slice — pull, list, delete over real podman
parent: 080-integration-tests
date: 2026-08-11
status: ready-for-agent
labels:
  - ready-for-agent
  - testing
  - backend
  - integration-tests
  - podman-integration
  - images
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

Integration tests covering the images domain over the real podman machine, reusing the harness from 080-01. Pull a tiny image (**busybox**), list images filtered by the run-scoped tag, and delete the pulled image. This proves the registry → SSH tunnel → podman socket path, including consuming the NDJSON pull-progress stream.

## Acceptance criteria

- [ ] test pulls `busybox` via `POST /api/machines/{id}/images/pull` and consumes the NDJSON progress stream to completion
- [ ] `GET /api/machines/{id}/images` returns the pulled image; assertions filter by the run tag so pre-existing images on the shared machine don't cause failures
- [ ] `DELETE /api/machines/{id}/images/{imageId}` removes it; a subsequent list no longer contains it
- [ ] the pulled image is registered with the run-scoped tag/prefix; per-test `t.Cleanup` deletes it; the suite sweep covers aborts
- [ ] test passes under `make test-integration`

## Blocked by

- 080-01-harness-tracer-bullet (needs the harness, authenticated client, and run-scoped tag helpers)

## Technical notes

- Use `busybox:latest` (or a pinned digest) for the smallest, fastest pull.
- List filtering: tag the pulled image or filter by repository/name prefix matching the `runID` so the assertions are independent of whatever else lives on the machine.
