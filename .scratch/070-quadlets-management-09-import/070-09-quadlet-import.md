---
title: Quadlet Import endpoint
parent: 070-quadlets-management
date: 2026-08-02
status: ready-for-agent
labels:
  - quadlets
  - backend
  - feature
  - podman-integration
author: vibe-agent
---

## Parent

070-quadlets-management — Implement Quadlet management endpoints (backend) against the podman service API

## Approach decision (resolved 2026-08-02)

**Chosen: narrow the contract to accept uploaded content.** `QuadletInstall` takes a tar body, not a remote path, so the request is changed to accept content (a list of `{name, resource, content}` entries, or a single quadlet) from the client and feed it to `QuadletInstall`. This maps cleanly onto the existing podman primitive with **no new infrastructure**. **Contract deviation:** `doku/description_api.md:1151-1191` documents a `{path, recursive}` request against a remote path — that must be updated to the content-based request shape as part of this slice.

## What to build

Implement `POST /api/machines/{id}/quadlets/import` end-to-end. Returns `{imported: [...], failed: [{path, error}]}` (201).

- Request is content-based (`QuadletImportRequest` with one or more `{name, resource, content}` entries); `Service.Import` tar-wraps each entry and calls `QuadletInstall`; maps `InstalledQuadlets`/`QuadletErrors` to the imported/failed arrays

`INVALID_PATH` / `VALIDATION_FAILED` (400) on bad input; `MACHINE_NOT_FOUND` (404); `CONNECTION_FAILED` (502).

## Acceptance criteria

- [ ] `Service.Import` accepts content entries, tar-wraps, and calls `QuadletInstall`
- [ ] `Handler.Import` returns `201` with `{imported, failed}`
- [ ] `doku/description_api.md:1151-1191` updated to the content-based request shape (contract deviation documented)
- [ ] `VALIDATION_FAILED` / `INVALID_PATH` (400) on bad input
- [ ] `CONNECTION_FAILED` on connection errors
- [ ] Partial-failure handling: per-file errors land in `failed[]`, successful ones in `imported[]`
- [ ] Service + handler unit tests covering all-success, partial-failure, bad-path, connection-failure

## Blocked by

- 070-03-quadlet-create (install/tar wrapping)

## Technical notes

- Podman reference: `QuadletInstall` at `~/repos/podman/pkg/domain/infra/tunnel/quadlet.go:26`; `QuadletInstallReport{InstalledQuadlets, QuadletErrors}`
- Tar framing reference: `createTarFromPath` / `createTarFromReader` at lines 115/161
- Contract update required: `doku/description_api.md:1151-1191` (replace `{path, recursive}` with content-based request)
