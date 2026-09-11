---
title: Quadlet CRUD vertical slice — create, list, get, update, delete over real podman
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

Integration tests covering the quadlet CRUD lifecycle over the real podman machine, reusing the harness from 080-01. Install a `.container` quadlet (tar-wrapped `QuadletInstall`), list quadlets filtered by run-scoped name, get content + metadata, update the content (remove + install-with-Replace), and delete. Proves the podman tunnel `QuadletInstall` / `QuadletList` / `QuadletPrint` / `QuadletRemove` path end-to-end through the SSH tunnel.

This slice is the foundation for the quadlet integration slices: it defines the shared `createQuadlet(t, machineID, name, resource, content string)` HTTP helper, the runnable-quadlet recipe, and the `registerQuadletsSweep()` + `podmanConnRun("quadlet", "rm", "-f", ...)` cleanup pattern reused by 080-09 / 080-10 / 080-12.

Run-scoped quadlet names use the established prefix `it-<runID>-<slug>` (e.g. `it-<runID>-crud`). The filename on the remote machine becomes `<name>.container`.

Runnable-quadlet recipe (reused by every slice that needs a unit systemd can actually start):

```ini
[Container]
Image=busybox
Exec=sh -c "sleep 3600"
```

`busybox` must be present on the machine — reuse `ensureBusyboxImage` (already in-package from the containers slice).

### Tripwire — the DELETE `stop` query param is a no-op (gap)

`QuadletService.Delete` currently ignores `stop` (`_ = stop // honoured once SSH-exec (070-10) is available`, `service.go:315`). SSH-exec has since landed, but the wiring was never retrofitted. Consequence: deleting a **running** quadlet without `force` returns `QUADLET_RESOURCE_RUNNING` (400) — the "stop-then-remove" path the `stop` param was meant to trigger does not exist. Pin this in code as a clearly-commented gap. The active behavioural assertion (running quadlet + no force → 400) lives in the lifecycle slice (080-09), where a started quadlet is available; this CRUD slice documents the gap and tests only the states it can reach (not-running quadlet: 204 happy path, 404 not-found, `force=true` harmless on a stopped unit).

## Acceptance criteria

- [ ] `POST /api/machines/{id}/quadlets` installs a run-scoped `.container` quadlet; `201` returned with `id` = name, `state: unknown`
- [ ] `GET /api/machines/{id}/quadlets` returns the created quadlet; assertions filter by the `it-<runID>-` name prefix so pre-existing quadlets don't cause failures
- [ ] `GET /api/machines/{id}/quadlets/{quadletId}` returns `200` with `content` + metadata (`resource`, `unit`, `path`)
- [ ] `PUT /api/machines/{id}/quadlets/{quadletId}` replaces content; a subsequent `GET` reflects the new content
- [ ] `DELETE /api/machines/{id}/quadlets/{quadletId}` returns `204`; a subsequent list excludes it
- [ ] `DELETE` on a missing quadlet returns `QUADLET_NOT_FOUND` (404)
- [ ] `POST` with a duplicate name returns `QUADLET_ALREADY_EXISTS` (409)
- [ ] shared `createQuadlet` helper + `registerQuadletsSweep()` are available to downstream slices
- [ ] the `stop`-param gap is documented in-code (tripwire comment); test passes under `make test-integration`

## Blocked by

- 080-01-harness-tracer-bullet (needs the harness, authenticated client, run-scoped tag/cleanup helpers — already merged)

## Technical notes

- Mirror `containers_test.go` for structure: `Test_Integration_Quadlets_CRUD` with `t.Run` subtests, `if t.Failed() { return }` bail between phases, local DTO structs (do NOT import the quadlets domain package), `require.Equalf` with body-echoing messages.
- URL construction: `suite.ServerURL + "/api/machines/" + machineID + "/quadlets/"` (trailing slash where chi requires it) and `/quadlets/{name}` for resource-level ops. The `quadletId` path param is the quadlet **name** (podman has no uuid).
- `QuadletCreateRequest` validates `resource` as `oneof=container pod image volume network` — use `container` for the runnable recipe.
- Cleanup: each created quadlet gets a `t.Cleanup` that `DELETE`s it via the API (fallback `podmanConnRun("quadlet", "rm", "-f", name)`); the suite sweep force-removes any name containing `RunID`.
- `podmanConnRun` already exists in `images_test.go` (same package) — do not redeclare it.
