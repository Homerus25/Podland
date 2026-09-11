---
title: Implement Quadlet management endpoints (backend) against the podman service API
parent: none
date: 2026-08-02
status: ready-for-agent
labels:
  - quadlets
  - backend
  - feature
  - podman-integration
  - epic
author: vibe-agent
---

## What to build

Implement the 11 Quadlet management endpoints for the Podman Management Web-App. All 11 routes are already wired in `cmd/server/main.go` under `/api/machines/{id}/quadlets` but every handler in `internal/domain/quadlets/handler.go` currently returns `501 Not Implemented`. The `QuadletService` is a 17-line stub with no methods; no DTOs or tests exist.

This epic is **backend-only** (no frontend exists yet), consistent with the 030-containers and 060-images work.

The podman repo is checked out under `~/repos/podman`. The reference tunnel implementation lives at `~/repos/podman/pkg/domain/infra/tunnel/quadlet.go` (methods are on `ContainerEngine`, **not** a separate QuadletEngine).

### Ruling: no SQLite persistence

Quadlet records are **not** persisted in our database. The service proxies podman directly, matching how `ContainerService` and `ImageService` work. This has consequences for the API contract (`doku/description_api.md:832-1191`) that each slice must honour:

- **`id`** — podman identifies quadlets by **name** only (no uuid). Use the quadlet name as the `id`. Documented deviation from the contract's "uuid".
- **`resource`** — derive from the quadlet filename suffix (`.container`/`.pod`/`.image`/`.volume`/`.network`/`.kube`/`.build`).
- **`state`** — parse the systemd `Status` string returned by `QuadletList` into `unknown|stopped|running|error` (heuristic substring matching).
- **`generation`, `createdAt`, `updatedAt`** — podman provides none of these. **Omit** these fields from responses (zero/absent). Documented deviation.

### Capability boundary (resolved)

The podman service API (tunnel `ContainerEngine`) exposes only **5** quadlet operations: `QuadletList`, `QuadletPrint` (raw file content), `QuadletInstall` (tar body upload), `QuadletRemove`, `QuadletExists`. The contract's Start/Stop/Restart, Logs, Generate, and Import endpoints have **no direct podman op** — their mechanisms are decided per slice:

- **Lifecycle (Start/Stop/Restart)** → `systemctl` over SSH (requires the new SSH-exec infra slice, 070-10)
- **Logs** → container logs via the podman API (reuse 060-04 streaming)
- **Generate** → run `pkg/systemd/quadlet` conversion in the backend; return content only (no auto-install)
- **Import** → narrow the contract to accept uploaded content (no remote-path semantics)

## Vertical Slices

Each slice cuts through service, handler, DTO, and tests end-to-end for one concern.

### 0. Prefactor — make QuadletService testable
**Issue**: 070-00-quadlet-service-testability
- Switch `QuadletService` to `machines.ConnectionManagerInterface`
- Add `NewQuadletServiceWithConcreteDeps` + `QuadletServiceInterface`
- Fix `cmd/server/main.go` wiring

**Status**: ready-for-agent
**Blocked by**: None

### 1. Quadlet List
**Issue**: 070-01-quadlet-list
- `Service.List` via `ContainerEngine.QuadletList`
- `QuadletResponse` DTO + mapping from `entities.ListQuadlet` (derive `resource`/`state`, `id`=name)
- `resource`/`name` query filters
- Tests

**Status**: ready-for-agent
**Blocked by**: 070-00

### 2. Quadlet Get
**Issue**: 070-02-quadlet-get
- `Service.Get` = `QuadletPrint` (content) + `QuadletList` filtered by name (metadata)
- Single-item DTO with `content`
- `QUADLET_NOT_FOUND` mapping
- Tests

**Status**: ready-for-agent
**Blocked by**: 070-01

### 3. Quadlet Create
**Issue**: 070-03-quadlet-create
- `Service.Create` via `QuadletInstall` (wrap request `content` as a tar body)
- `QuadletCreateRequest` DTO (`name`, `resource`, `content`)
- `QUADLET_ALREADY_EXISTS` (409) mapping
- Tests

**Status**: ready-for-agent
**Blocked by**: 070-01

### 4. Quadlet Delete
**Issue**: 070-04-quadlet-delete
- `Service.Delete` via `QuadletRemove`
- `stop`/`force` query params → `QuadletRemoveOptions{Force, ReloadSystemd}`
- `QUADLET_RESOURCE_RUNNING`, `QUADLET_NOT_FOUND` mapping
- Tests

**Status**: ready-for-agent
**Blocked by**: 070-01

### 5. Quadlet Update
**Issue**: 070-05-quadlet-update
- `Service.Update` = `QuadletRemove` + `QuadletInstall(Replace:true)` of new content
- `QUADLET_NOT_FOUND` on missing source
- Tests

**Status**: ready-for-agent
**Blocked by**: 070-03, 070-04

### 6. Quadlet Lifecycle (Start/Stop/Restart)
**Issue**: 070-06-quadlet-lifecycle
- Run `systemctl start|stop|restart <unit>` over SSH via `SSHExecInterface` (070-10); resolve quadlet→unit name.

**Status**: ready-for-agent
**Blocked by**: 070-02, 070-10

### 7. Quadlet Logs (NDJSON streaming)
**Issue**: 070-07-quadlet-logs
- Stream logs of the managed container via `ContainerEngine` (reuse 060-04 NDJSON pattern); resolve quadlet→container.

**Status**: ready-for-agent
**Blocked by**: 070-02

### 8. Quadlet Generate (from container/pod)
**Issue**: 070-08-quadlet-generate
- Pull container/pod inspect via podman API, run `pkg/systemd/quadlet` conversion **in the backend**, return generated `content`.
- Fits the contract response shape (`content`, `state: unknown`); no auto-start.

**Status**: ready-for-agent
**Blocked by**: 070-01

### 9. Quadlet Import (content-based)
**Issue**: 070-09-quadlet-import
- Narrow the contract: accept content from the client, feed to `QuadletInstall`. **Contract update required** in `doku/description_api.md:1151-1191`.

**Status**: ready-for-agent
**Blocked by**: 070-03

### 10. SSH-exec infrastructure
**Issue**: 070-10-ssh-exec-infrastructure
- Add SSH-exec capability (`SSHExecInterface`) to the connection layer; first consumer is 070-06.

**Status**: ready-for-agent
**Blocked by**: None

## Technical Context

- **Routing**: Already set up in `cmd/server/main.go:163-178` (`/quadlets`, `/quadlets/import`, `/{quadletId}` CRUD + `/start`, `/stop`, `/restart`, `/logs`, `/generate`)
- **Current State**: `QuadletService` stub; all 11 handlers return `501`; no DTOs/tests
- **Podman Reference** (`~/repos/podman/pkg/domain/infra/tunnel/quadlet.go`, on `ContainerEngine`):
  - `QuadletList(ctx, entities.QuadletListOptions) ([]*entities.ListQuadlet, error)` — line 77
  - `QuadletPrint(ctx, name string) (string, error)` — line 104 (raw file content)
  - `QuadletInstall(ctx, []string, entities.QuadletInstallOptions) (*entities.QuadletInstallReport, error)` — line 26 (tar body)
  - `QuadletRemove(ctx, []string, entities.QuadletRemoveOptions) (*entities.QuadletRemoveReport, error)` — line 108
  - `QuadletExists(ctx, nameOrID) (*entities.BoolReport, error)` — line 21
- **Entities**: `~/repos/podman/pkg/domain/entities/quadlet.go` — `ListQuadlet{Name, UnitName, Path, Status, App, Pod}`, `QuadletInstallReport{InstalledQuadlets, QuadletErrors}`, `QuadletRemoveReport{Removed, Errors}`
- **Error Codes**: All 6 already exist in `internal/errors/errors.go:72-79` (`QUADLET_NOT_FOUND`, `QUADLET_ALREADY_EXISTS`, `QUADLET_RESOURCE_RUNNING`, `QUADLET_ALREADY_RUNNING`, `QUADLET_NOT_RUNNING`, `QUADLET_GENERATION_FAILED`)
- **Connection Management**: Use existing `ConnectionManager.GetConnection()` (same pattern as `ContainerService`/`ImageService`). The connection layer is a **podman-socket tunnel only** — it does NOT expose SSH exec/session capability.
- **API Contract**: See `doku/description_api.md:832-1191` for request/response shapes and error codes
- **Established Patterns**: Mirror `internal/domain/images/` (service interface + concrete-deps adapter + DTO mapping + handler tests)

## Acceptance criteria

- [ ] Quadlet list returns real quadlets from remote machines with derived `resource`/`state`
- [ ] Get returns a quadlet's content + metadata
- [ ] Create installs a new quadlet from content; 409 on name conflict
- [ ] Delete removes a quadlet, respecting `force`
- [ ] Update replaces quadlet content
- [ ] Generate, Lifecycle, Logs, and Import behave per their (resolved) slice decisions
- [ ] Contract field deviations (`id`=name; `generation`/`createdAt`/`updatedAt` omitted) are applied consistently and documented in code
- [ ] Proper error handling for connection failures and API errors
- [ ] Consistent API response format with containers/images
- [ ] Comprehensive test coverage following the images/containers test patterns

## Blocked by

None - can start immediately with slice 0 (prefactor)
