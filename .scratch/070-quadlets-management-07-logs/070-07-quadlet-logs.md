---
title: Quadlet Logs endpoint (streaming)
parent: 070-quadlets-management
date: 2026-08-02
status: ready-for-agent
labels:
  - quadlets
  - backend
  - feature
  - podman-integration
  - streaming
author: vibe-agent
---

## Parent

070-quadlets-management — Implement Quadlet management endpoints (backend) against the podman service API

## Source decision (resolved 2026-08-02)

**Chosen: container logs via the podman API.** Stream logs of the **managed container** using `ContainerEngine` logs, reusing the 060-04 NDJSON streaming pattern. This returns what users most often want (the application's stdout/stderr) and needs no new infrastructure. Requires resolving quadlet-name → managed container name.

## What to build

Implement `GET /api/machines/{id}/quadlets/{quadletId}/logs` end-to-end. Supports query params `follow`, `since`, `tail`.

- Non-streaming (`follow=false`, default): returns `{id, logs}` as a single multiline string
- Streaming (`follow=true`): `Content-Type: application/x-ndjson`, each line a `{timestamp, level, message}` object — mirror the 060-04 progress-event streaming approach and `OPERATION_CANCELLED` on client disconnect

Resolve quadlet name → managed container name (via List/`UnitName`, or by listing containers whose name derives from the unit) before calling `ContainerEngine` logs. `QUADLET_NOT_FOUND` (404) when the quadlet doesn't exist.

## Acceptance criteria

- [ ] `Service.GetLogs` resolves the managed container and returns logs (and a streaming variant with a callback writer, mirroring `ImageService.PullWithProgress`)
- [ ] `Handler.GetLogs` returns `{id, logs}` for non-streaming and NDJSON for `follow=true`
- [ ] `follow`/`since`/`tail` query params honoured
- [ ] Client-disconnect → `OPERATION_CANCELLED`
- [ ] `QUADLET_NOT_FOUND` (404) when the quadlet doesn't exist
- [ ] `CONNECTION_FAILED` on connection errors
- [ ] Service + handler unit tests covering streaming + non-streaming + not-found + cancellation

## Blocked by

- 070-02-quadlet-get (quadlet existence + container-name resolution)

## Technical notes

- Streaming pattern to mirror: `internal/domain/images/service.go` `PullWithProgress` + `progressAdapter`; NDJSON writer in `internal/domain/images/handler.go`
- Logs via `tunnel.ContainerEngine` logs; resolve quadlet→container via `QuadletList` `UnitName` or container-name convention
- Contract: `doku/description_api.md:1072-1109`
