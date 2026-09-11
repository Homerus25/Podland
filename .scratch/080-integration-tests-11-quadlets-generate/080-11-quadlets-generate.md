---
title: Quadlet Generate vertical slice — generate from a real container/pod
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
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

Integration test covering `POST /api/machines/{id}/quadlets/{quadletId}/generate`. The service inspects an existing container (or pod) via the podman API and converts it into a `.container` quadlet file **locally in the backend** (not via a remote podman primitive). Returns `200` with `{…, state: "unknown", content}`. This slice proves the inspect + local-conversion path.

Create a throwaway run-scoped container (reuse the containers-slice `createContainer` helper / busybox), then POST `/generate` with `{resourceId, resourceType: "container"}`. Assert the returned `content` is a non-empty string containing a `[Container]` section and referencing the source image. No files are installed on the remote machine.

Note: the `{quadletId}` path param is effectively ignored by the service (the body's `resourceId` is the source of truth) — pass any value.

## Acceptance criteria

- [ ] `POST .../generate` with a real container returns `200` and a `content` string containing `[Container]`
- [ ] generated `state` is `unknown`; no quadlet is installed on the remote machine (list unchanged)
- [ ] `resourceType: "pod"` path covered if practical (create a throwaway pod); otherwise skipped with a clear reason
- [ ] generate for a missing container returns `CONTAINER_NOT_FOUND` (404)
- [ ] throwaway container/pod cleaned up via `t.Cleanup` + sweep; test passes under `make test-integration`

## Blocked by

- 080-01-harness-tracer-bullet (needs the harness + authenticated client — already merged). Independent of the other quadlet slices.

## Technical notes

- Reuse `ensureBusyboxImage` + the `createContainer` helper from `containers_test.go` (same package) to make a source container. The container need not be running — inspect works on a created container.
- The conversion runs via the backend's own rendering (`generateContainerQuadlet`), not podman's `pkg/systemd/quadlet`; assert structurally (contains `[Container]`, contains the image name) rather than for an exact string.
- The `{quadletId}` path param quirk is documented in `070-08`; pass the container name or any placeholder.
