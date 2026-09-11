---
title: Image Pull — basic (POST /api/machines/{id}/images/pull)
parent: 060-image-management
date: 2026-07-30
status: completed
labels:
  - completed
  - images
  - backend
  - service-layer
  - podman-integration
author: vibe-agent
---

## Parent

060-image-management — Implement Image Management endpoints using official podman implementation

## What to build

Implement a working **image pull** path end-to-end: an authenticated user submits an image reference and the backend pulls it from a registry onto the target machine, returning a simple JSON result with the pulled image reference(s).

This is the **basic** slice — the handler returns a normal JSON response (the pulled image IDs/references) rather than streaming progress. Streaming progress and client-abort handling are added in the follow-up slice 060-04, which will refactor the response mechanism. Delivering a working pull first keeps this tracer bullet narrow and independently verifiable.

The user calls `POST /api/machines/{id}/images/pull` with `{ "image": "...", "credentials": {...}, "platform": "..." }`.

This slice covers: the `Service.Pull` method using the official podman tunnel `ImageEngine.Pull`, the `ImagePullRequest` DTO, the `Handler.Pull` implementation (non-streaming JSON response), `IMAGE_PULL_FAILED` handling, and unit tests.

## Acceptance criteria

- [ ] `Service.Pull(ctx, userID, machineID, request)` calls the official podman `ImageEngine.Pull` and returns the pulled image reference(s)
- [ ] `ImagePullRequest` DTO exists with `image` (required, validated), optional `credentials` (`username`/`password`), and optional `platform`, using `go-playground/validator` struct tags consistent with `ContainerCreateRequest`
- [ ] Service uses `ConnectionManager.GetConnection()` / `Release()` following the container service pattern
- [ ] When the pull fails, the service returns `errors.ErrImagePullFailed` (500, `IMAGE_PULL_FAILED`); invalid request bodies map to `INVALID_REQUEST` and validation failures to `VALIDATION_FAILED`
- [ ] `Handler.Pull` extracts user ID from context and machine ID from the URL, decodes and validates the request body, and returns a JSON response (e.g. `{ "images": [...] }` or the pulled references) — **non-streaming** for this slice
- [ ] `Handler.Pull` uses `slog` logging consistent with `ContainerHandler.Create`
- [ ] Unit tests for `Service.Pull` cover: connection failure, missing image parameter, pull-failure mapping
- [ ] Unit tests for `Handler.Pull` cover: missing user ID, missing machine ID, invalid/empty request body, validation failure, service-error path
- [ ] `go build ./...`, `go vet ./...`, and `go test ./...` all pass

## Blocked by

- 060-00-image-service-testability (needed for the testable service and mock injection)

## Technical notes

- Reference podman implementation: `~/repos/podman/pkg/domain/infra/tunnel/images.go:115` — `func (ir *ImageEngine) Pull(_ context.Context, rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error)`. It returns `entities.ImagePullReport{Images: pulledImages}`.
- Construct `entities.ImagePullOptions` from the request: map `credentials` to `Username`/`Password` and `platform` to `OS`/`Arch`/`Variant` as applicable. Note the tunnel `Pull` rejects `OciDecryptConfig` for remote clients, so do not set it.
- Handler pattern to follow: `internal/domain/containers/handler.go` `Create` (lines 149-205) for body decoding, `validator.New().Struct(request)`, error responses, and JSON encoding
- DTO/validation pattern: `internal/domain/containers/dto.go` `ContainerCreateRequest`
- API request/response shape: `doku/description_api.md` lines 731-807 (the streaming event shapes there are implemented in slice 060-04; this slice only needs the request body and a simple success response)
- **Intentional scope boundary**: this slice deliberately does NOT implement streaming. Slice 060-04 converts the response to NDJSON and adds progress + abort.
