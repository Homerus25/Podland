---
title: Container logs endpoint (non-streaming tracer bullet)
parent: 035-container-logs
date: 2026-08-02
status: completed
labels:
  - completed
  - containers
  - backend
  - feature
  - podman-integration
author: vibe-agent
---

## Parent

035-container-logs — Implement Container Logs endpoint (backend) against the podman service API

## What to build

Land the end-to-end path for fetching a container's logs **without** streaming first — a narrow but complete tracer bullet through routing → handler → service → podman → response.

`GET /api/machines/{id}/containers/{containerId}/logs` returns the container's recent stdout+stderr as a single JSON object `{"id": "<containerId>", "logs": "<multiline string>"}`. It honours the `since` (ISO8601) and `tail` (integer) query params. `follow` is accepted but defaults to `false`; a `follow=true` request in this slice behaves like non-follow (it just returns once, or is deferred to 035-02 — see acceptance criteria).

The service obtains a podman connection via the existing `ConnectionManager`/`tunnel.ContainerEngine` pattern, calls `ContainerEngine.ContainerLogs(ctx, []string{containerId}, entities.ContainerLogsOptions{…})` with `StdoutWriter`/`StderrWriter` set to a buffer, and returns the concatenated text. Map a missing/invalid container to `CONTAINER_NOT_FOUND` (404) and connection failures to `CONNECTION_FAILED` (400), mirroring the other container endpoints.

This slice also **adds the container-logs section to `doku/description_api.md`**, modelled on the quadlet-logs section (lines 1072–1109) but with `CONTAINER_NOT_FOUND` instead of `QUADLET_NOT_FOUND` and `id` = the container id.

## Acceptance criteria

- [ ] `GET /api/machines/{id}/containers/{containerId}/logs` is routed in `cmd/server/main.go` inside the existing `/{containerId}` group
- [ ] `ContainerServiceInterface` gains a `GetLogs` method; `ContainerService.GetLogs` fetches logs via `ContainerEngine.ContainerLogs` using `StdoutWriter`/`StderrWriter`
- [ ] `ContainerHandler.Logs` parses `since`/`tail` (and `follow`) query params, calls the service, and returns `{"id", "logs"}` with `200`
- [ ] `since` (ISO8601 → `time.Time`) and `tail` (integer) are honoured; invalid values are ignored or rejected with a `400` (pick one and be consistent with the other container handlers)
- [ ] `follow=true` is accepted without error in this slice (it may behave as non-follow; full streaming is 035-02)
- [ ] `CONTAINER_NOT_FOUND` (404) returned for an unknown container; `CONNECTION_FAILED` (400) for connection errors; auth/permission errors consistent with the other container endpoints
- [ ] Container-logs section added to `doku/description_api.md` (non-streaming response shape + error codes)
- [ ] Unit tests for `ContainerService.GetLogs` (success, container-not-found, connection-failure) and `ContainerHandler.Logs` (param parsing, success, error mapping) using the existing `MockConnectionManager`
- [ ] `go build ./...`, `go vet ./...`, and `go test ./...` are green

## Blocked by

- 035-00-streaming-helper (the streaming primitives are needed by 035-02, but 035-01 should land on top of the refactored images package to avoid merge churn — if 035-00 is still open, this slice may proceed in parallel and rebase)

## Technical notes

- Podman API: `ContainerLogs(_ context.Context, nameOrIDs []string, opts entities.ContainerLogsOptions) error` at `~/repos/podman/pkg/domain/infra/tunnel/containers.go:550`. Pass `[]string{containerID}`. Set `opts.StdoutWriter` and `opts.StderrWriter` to a `bytes.Buffer` (or two buffers merged for the response). Use `opts.Since` (`time.Time`), `opts.Tail` (`int64`), `opts.Timestamps` if you want timestamps embedded in the log text.
- `entities.ContainerLogsOptions` struct: `~/repos/podman/pkg/domain/entities/containers.go:257`.
- Existence check: `ContainerLogs` will error on an unknown id — map that to `ErrContainerNotFound`. Alternatively reuse the existing `ContainerService.Get` (`ContainerInspect`) to pre-validate, but prefer letting the logs call surface the error to avoid a double round-trip.
- Pattern to mirror: `internal/domain/containers/handler.go` `Get`/`Delete` for handler structure, and `internal/domain/containers/service.go` `Get` for the connection/engine setup.
- The API contract section to add sits alongside the other container endpoints in `doku/description_api.md` (Container Endpoints starts ~line 446). Mirror the quadlet-logs block at 1072–1109.
