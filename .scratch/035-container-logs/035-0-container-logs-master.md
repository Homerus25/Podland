---
title: Implement Container Logs endpoint (backend) against the podman service API
parent: none
date: 2026-08-02
status: completed
labels:
  - completed
  - containers
  - backend
  - feature
  - podman-integration
  - streaming
  - epic
author: vibe-agent
---

## What to build

Implement `GET /api/machines/{id}/containers/{containerId}/logs` end-to-end for the Podman Management Web-App. This is **backend-only** (no container-logs UI exists yet), consistent with the 030-containers and 060-images work.

This endpoint does **not** exist yet — it is neither routed nor in the API contract. The only logs endpoint today is the (still-unimplemented) quadlet logs at 070-07, which established the dual-mode contract this epic reuses. The container list/get/create/start/stop/delete endpoints (030–034) are all implemented, so this is a clean addition to a working domain.

### Ruling: dual-mode response (matches quadlet logs 070-07)

- **Non-streaming** (`follow=false`, default): return `{"id", "logs"}` where `logs` is a single multiline string (stdout+stderr concatenated).
- **Streaming** (`follow=true`): `Content-Type: application/x-ndjson`, each line a `{"timestamp", "level", "message"}` object — mirror the 060-04 image-pull NDJSON pattern and emit `OPERATION_CANCELLED` on client disconnect.

Query params: `follow` (bool), `since` (ISO8601), `tail` (int, number of trailing lines).

### Ruling: container logs, not quadlet resolution

Unlike 070-07 (which must resolve quadlet-name → managed container), this endpoint receives a `containerId` directly, so there is **no name-resolution step**. Existence is validated by the podman logs call itself (or a lightweight inspect), mapping to `CONTAINER_NOT_FOUND`.

## Vertical Slices

Each slice cuts through service, handler, DTO, routing, and tests end-to-end for one concern.

### 0. Prefactor — extract shared NDJSON streaming helper
**Issue**: 035-00-streaming-helper
- Extract the generic HTTP/NDJSON streaming mechanics out of `internal/domain/images/handler.go` into a reusable `internal/streaming` package; refactor image-pull to use it. No behavior change.

**Status**: ready-for-agent
**Blocked by**: None

### 1. Container logs (non-streaming tracer bullet)
**Issue**: 035-01-container-logs-non-streaming
- Wire `GET .../containers/{containerId}/logs`; `Service.GetLogs` via `ContainerEngine.ContainerLogs`; return `{id, logs}`; honour `since`/`tail`; add the container-logs section to `doku/description_api.md`.

**Status**: ready-for-agent
**Blocked by**: 035-00

### 2. Container logs (streaming / follow)
**Issue**: 035-02-container-logs-streaming
- `follow=true` → NDJSON `{timestamp, level, message}` per line; client-disconnect → `OPERATION_CANCELLED`; reuse `internal/streaming` from 035-00.

**Status**: ready-for-agent
**Blocked by**: 035-01

## Technical Context

- **Routing**: The `/{containerId}` group in `cmd/server/main.go` currently registers Get/Start/Stop/Delete only. Add `r.Get("/logs", containerHandler.Logs)` inside it (lines ~148–153). A `/logs` route already exists on the **quadlet** group (line 175, pointing at `quadletHandler.GetLogs`) — do not touch it.
- **Podman Reference** (`~/repos/podman/pkg/domain/infra/tunnel/containers.go:550`):
  - `ContainerLogs(_ context.Context, nameOrIDs []string, opts entities.ContainerLogsOptions) error`
  - Writes log lines to `opts.StdoutWriter` / `opts.StderrWriter` (`io.Writer`); honours `Follow`, `Since`, `Until`, `Tail`, `Timestamps`.
  - **Cancellation caveat**: podman runs `containers.Logs(ic.ClientCtx, …)` in a goroutine driven by its own internal `context.Background()` select loop. The handler's cancelable request context alone does **not** propagate into the follow loop — see 035-02 for the seam.
  - `entities.ContainerLogsOptions` struct: `~/repos/podman/pkg/domain/entities/containers.go:257` (`StdoutWriter`, `StderrWriter`, `Follow`, `Since`, `Until`, `Tail`, `Timestamps`).
- **Connection Management**: Use the existing `ConnectionManager` → `tunnel.ContainerEngine{ClientCtx: connCtx}` pattern already in `internal/domain/containers/service.go` (`List`/`Get`/`Start`/etc.).
- **Error Codes**: `ErrContainerNotFound` (404), `ErrConnectionFailed` (400), `ErrOperationCancelled` (504) already exist in `internal/errors/errors.go`. No new codes needed.
- **API Contract**: There is **no** container-logs section in `doku/description_api.md` today — 035-01 adds one (model it on the quadlet-logs section at `doku/description_api.md:1072-1109`, swapping `QUADLET_NOT_FOUND` for `CONTAINER_NOT_FOUND`).
- **Established Streaming Pattern**: `internal/domain/images/handler.go` `Pull` (NDJSON headers, `http.Flusher`, disconnect channel, cancelable context, event loop). The pull-specific event types (`PullProgressEvent`) are **not** reused — log lines use `{timestamp, level, message}`.
- **Testability**: `ContainerService` already depends on `machines.ConnectionManagerInterface` and has a mock (`internal/testutils/mock_services.go` `MockConnectionManager`); `ContainerServiceInterface` is already defined and just needs a `GetLogs` method added.

## Acceptance criteria

- [ ] `GET /api/machines/{id}/containers/{containerId}/logs` returns container logs from the remote machine
- [ ] Non-streaming mode returns `{id, logs}` as a single multiline string
- [ ] Streaming mode (`follow=true`) emits NDJSON `{timestamp, level, message}` lines and `OPERATION_CANCELLED` on client disconnect
- [ ] `follow`/`since`/`tail` query params are honoured
- [ ] `CONTAINER_NOT_FOUND` (404) and `CONNECTION_FAILED` (400) are returned correctly
- [ ] Container-logs section added to `doku/description_api.md`
- [ ] NDJSON streaming mechanics shared with the images package via `internal/streaming`
- [ ] Comprehensive unit tests for service + handler (streaming, non-streaming, not-found, cancellation)
- [ ] `go build ./...`, `go vet ./...`, and `go test ./...` are green

## Blocked by

None - can start immediately with slice 0 (prefactor)
