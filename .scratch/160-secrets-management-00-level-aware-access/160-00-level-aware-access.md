---
title: Level-aware machine access middleware + retrofit mutating routes
parent: 160-secrets-management
date: 2026-08-27
status: closed
labels:
  - ready-for-agent
  - backend
  - secrets
  - feature
author: vibe-agent
---

## Parent

160-secrets-management — Implement Secret management endpoints (backend) against the podman service API

## What to build

Make machine access checks level-aware, then apply them to every mutating route. Today `RequireMachineAccess` (`internal/middleware/machine_access.go:54-96`) only asks `accessRepo.HasAccess(machineID, userID)` (`internal/models/machine_access_repository.go:134-144`, a `SELECT COUNT(*)`), so **any** granted user — including `read` — can call mutating container/image/quadlet endpoints. The secrets feature needs `read` = view metadata, `write` = change, `admin` = reveal values, and that story must hold app-wide.

Two pieces:

1. **Repo**: add `GetPermission(machineID, userID) (string, error)` to the machine access repository (empty string when no row) plus a `HasAtLeast(required, actual string) bool` ordering helper in `internal/models` (`read` < `write` < `admin`). Unit-test the ordering helper.

2. **Middleware**: add `RequireMachinePermission(accessRepo, required string)` mirroring `RequireMachineAccess` (machine lookup → 404, permission fetch → `ErrAccessDenied` 403 when below level). Keep `RequireMachineAccess` as the read-level entry for the `{id}` subtree; add `RequireMachinePermission(models.PermissionWrite)` to the **mutating** route groups in `internal/app/app.go`:

   - containers: create, delete, start, stop, restart
   - images: pull, delete
   - quadlets: create, import, update, delete, start, stop, restart

   Read-only routes stay at subtree level: lists, gets, logs, generate, events.

Follow the frontend counterpart later (170 gates buttons); this slice is backend-only enforcement. Existing integration tests use write/admin users, so no test rewrites expected — but add tripwires for the new behaviour.

## Acceptance criteria

- [x] `GetPermission` returns the stored level ('' when absent) and is unit-tested
- [x] Level ordering helper enforces read < write < admin
- [x] `RequireMachinePermission` returns 404 `MACHINE_NOT_FOUND` for unknown machines, 403 `ACCESS_DENIED` for insufficient level — unit tests for both
- [x] All listed mutating routes reject `read`-level users with 403 and still accept `write`/`admin` (integration or handler tests)
- [x] Read-only routes (list/get/logs/generate/events) remain callable with `read`
- [x] Integration tripwire: a `read` user cannot delete a container, pull an image, or stop a quadlet

## Blocked by

- None

## Technical notes

- Schema already stores the level: `machine_access.permission CHECK(permission IN ('read','write','admin'))` (`backend/migrations/001_initial_schema.sql:27-35`); constants at `internal/models/machine.go:9-13`
- Owner implicitly holds `admin` via the creation-time access row (`internal/domain/machines/service.go:162-168`)
- Place the ordering helper in `internal/models` next to the permission constants so middleware and handlers share one source of truth

## Implementation notes (2026-08-28)

- "containers: … restart" in the route list was a spec slip: no container restart route exists (only quadlets have restart), so there was nothing to gate.
- Machines-domain mutations (PUT/DELETE machine, access grant/revoke) intentionally stay at subtree level with their existing owner checks, per the enumerated route list; the secrets slice (160-01+) adds level gates where it needs them.
