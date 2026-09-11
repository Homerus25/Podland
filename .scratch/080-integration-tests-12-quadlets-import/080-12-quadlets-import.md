---
title: Quadlet Import vertical slice — bulk install + partial failure
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

Integration test covering `POST /api/machines/{id}/quadlets/import`. Accepts a content-based request (one or more `{name, resource, content}` entries), tar-wraps each, and calls `QuadletInstall` per entry. Returns `201` with `{imported: [...], failed: [{name, error}]}`. This slice proves the multi-file install + partial-failure path.

POST two valid run-scoped `.container` quadlets plus one entry that collides with an existing name (or is otherwise invalid) to force a partial failure. Assert that the valid entries appear in `imported[]` and the bad entry appears in `failed[]` with a non-empty `error`.

## Acceptance criteria

- [ ] `POST .../quadlets/import` with multiple valid entries returns `201`; each appears in `imported[]` with `id` = name
- [ ] an entry whose name collides with a pre-existing quadlet lands in `failed[]` with a non-empty `error`; valid siblings still land in `imported[]` (partial success)
- [ ] imported quadlets are visible via `GET .../quadlets` (run-scoped filtered)
- [ ] `VALIDATION_FAILED` (400) on a request with no entries / invalid `resource`
- [ ] all imported quadlets cleaned up via `t.Cleanup` + sweep; test passes under `make test-integration`

## Blocked by

- 080-08-quadlets-crud (reuses the create-quadlet content recipe, the `createQuadlet` helper for seeding a collision target, and the sweep pattern)

## Technical notes

- To force the collision/partial-failure: first `POST /quadlets` to install a quadlet named `it-<runID>-collide`, then include an import entry with the same name — it should land in `failed[]`.
- `QuadletImportRequest` validates `Quadlets` as `required,min=1,dive`; each entry's `resource` is `oneof=container pod image volume network`.
- Mirror the CRUD slice's cleanup: `t.Cleanup` deletes each imported quadlet by name; the sweep force-removes anything containing `RunID`.
