---
title: Quadlet Logs vertical slice — non-streaming + NDJSON streaming
parent: 080-integration-tests
date: 2026-08-13
status: ready-for-agent
labels:
  - ready-for-agent
  - testing
  - backend
  - integration-tests
  - podman-integration
  - quadlets
  - streaming
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

Integration tests covering `GET /api/machines/{id}/quadlets/{quadletId}/logs`. The service resolves the quadlet to its managed container and streams that container's logs via the podman API. This slice proves the quadlet→container resolution + NDJSON streaming path.

Two modes:

- **Non-streaming** (`follow=false`, default): returns `200` with `{id, logs}` as a single multiline string.
- **Streaming** (`follow=true`): `Content-Type: application/x-ndjson`, each line a `{timestamp, level, message}` object. Mirror the images-slice streaming helper (`pullImageStreamToCompletion`): hand-set the `Authorization: Bearer` header and use a no-timeout `http.Client` (the default `AuthenticatedClient` has a 30s timeout that will cut off a follow stream).

The quadlet must be **running** so it has a managed container producing logs — create + start one via the helpers from 080-08 / 080-09. Client disconnect mid-stream must yield `OPERATION_CANCELLED`.

## Acceptance criteria

- [ ] non-streaming `GET .../logs` on a running quadlet returns `200` `{id, logs}` with non-empty `logs`
- [ ] streaming `GET .../logs?follow=true` returns `application/x-ndjson`; lines decode into `{timestamp, level, message}`
- [ ] `since` (RFC3339) and `tail` query params accepted
- [ ] client disconnect during `follow=true` → `OPERATION_CANCELLED` (server logs it; the request context cancels)
- [ ] `GET .../logs` on a missing quadlet returns `QUADLET_NOT_FOUND` (404)
- [ ] `GET .../logs` on a quadlet whose managed container doesn't exist returns `QUADLET_NOT_FOUND` (404)
- [ ] quadlet is stopped + cleaned up via `t.Cleanup` + sweep; test passes under `make test-integration`

## Blocked by

- 080-09-quadlets-lifecycle (needs a started quadlet producing a managed container; `follow=true` is only meaningful with a live container)

## Technical notes

- Streaming client pattern: copy `pullImageStreamToCompletion` from `images_test.go` — it sets the Bearer header manually, uses `&http.Client{}` (no timeout), and scans newline-delimited JSON. Do not use `suite.AuthenticatedClient()` for the streaming case.
- Quadlet→container resolution is done by the service (`resolveForLogs`); you only need the quadlet **name** for the HTTP path.
- To guarantee non-empty logs, start the quadlet and wait for `running`, then give the container a moment to emit output before the non-streaming GET. The default runnable recipe (`sleep 3600`) won't emit stdout by itself — for this slice prefer a logging recipe such as `Exec=sh -c "while true; do echo hi; sleep 1; done"` so the assertions are meaningful.
