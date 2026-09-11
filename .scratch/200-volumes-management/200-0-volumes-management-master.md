---
title: Volume management for machines (list, inspect, create, delete, prune, export, populate)
parent: none
date: 2026-08-30
status: ready-for-agent
labels:
  - volumes
  - backend
  - frontend
  - feature
  - podman-integration
  - epic
author: vibe-agent
---

## Problem Statement

Podland users can manage containers, images, quadlets and secrets on their registered machines through the web UI, but volumes are invisible. Volumes hold the persistent data of containerized workloads, yet today a user who wants to see what volumes exist, create one for a new service, clean up orphaned volumes, back up a volume's contents, or seed a volume with configuration files must SSH into the machine and run podman CLI commands by hand. Shared users without direct SSH access cannot do any of this at all. There is no way to see which volumes are consuming disk space on a machine, and no safe, permission-aware way to move data in or out of a volume.

## Solution

Add complete volume management to Podland, scoped per machine, following the established containers pattern: a new backend domain that proxies the podman Service API over the existing SSH tunnel connection, REST routes under `/api/machines/{id}/volumes`, and a new "Volumes" tab on the machine detail page.

Users get: a searchable list of all volumes (named and anonymous) with metadata; an inspect drawer showing full config, labels, driver, options and creation time; creation with the full set of podman options (name, labels, driver, driver options); deletion with an explicit force flag for in-use volumes; a prune action for unused volumes; tar export as a browser download; and populate, which uploads either individual files (server assembles a tar) or a ready-made tar archive into the volume.

Permissions follow the machine-access model: read-level users can list, inspect and export; write-level users can additionally create, delete, populate and prune. Volume data is never persisted in Podland's database — the backend is a pure proxy to podman, exactly like containers and secrets.

## User Stories

1. As a machine owner, I want to see a list of all volumes on my machine, so that I know what persistent data exists and what is consuming disk space.
2. As a machine owner, I want to see at a glance whether a volume is named or anonymous, so that I can distinguish deliberate volumes from container-internal ones.
3. As a machine owner, I want to see when each volume was created, so that I can correlate volumes with workloads I set up over time.
4. As a machine owner, I want to inspect a volume's full details (driver, options, labels, mountpoint), so that I can debug mounting problems without SSH access.
5. As a machine owner, I want to create a named volume, so that I can attach it to a container later.
6. As a machine owner, I want to create an anonymous volume and learn its generated name, so that workflows requiring anonymous volumes still work.
7. As a power user, I want to create a volume with a non-default driver and driver options, so that I can use specialized storage backends supported by podman.
8. As a power user, I want to attach labels to a volume at creation time, so that I can organize and filter volumes by purpose or owning workload.
9. As a machine owner, I want to delete a volume I no longer need, so that I can reclaim disk space on the machine.
10. As a machine owner, I want deletion to refuse when a volume is in use by a container, so that I cannot accidentally break a running workload.
11. As a machine owner, I want an explicit, separately-confirmed force option that stops dependent containers before deleting, so that I can clean up stuck or abandoned volumes deliberately.
12. As a write-level shared user, I want to delete volumes on a machine shared with me, so that I can help maintain it.
13. As a machine owner, I want to prune all unused volumes in one action with a clear confirmation, so that I can reclaim space after a big cleanup without deleting volumes one-by-one.
14. As a machine owner, I want to see a report of what prune removed, so that I have a record of the reclaimed volumes.
15. As a machine owner, I want to export a volume as a tar archive downloaded to my browser, so that I can back up or migrate volume data.
16. As a read-level shared user, I want to export a volume, so that I can back up data from a machine I can inspect but not modify.
17. As a machine owner, I want to populate a volume by picking several files in a file picker, so that I can seed configuration or static assets without building a tar myself.
18. As a power user, I want to populate a volume by uploading a ready-made tar archive, so that I can transfer complex directory trees in one step.
19. As a machine owner, I want relative paths preserved when I upload multiple files, so that the directory structure inside the volume matches what I selected.
20. As a machine owner, I want populate to merge uploaded files into the volume (overwriting only same-named paths), so that the behavior is predictable and matches podman semantics.
21. As a machine owner, I want the UI to clearly state that populate merges rather than replaces, so that I do not expect stale files to disappear.
22. As a machine owner, I want a clear error when my upload exceeds the configured size limit, so that I know to split or shrink it instead of guessing.
23. As a read-level shared user, I want list, inspect and export to work for me, so that I can work with a machine's state I am allowed to read.
24. As a read-level shared user, I want mutating actions (create, delete, populate, prune) to be hidden or disabled in the UI and rejected by the API, so that I never get halfway through an action only to be blocked.
25. As a machine owner, I want a per-machine "Volumes" tab alongside Containers, Images, Quadlets and Secrets, so that volume management lives where I already manage the machine.
26. As a machine owner, I want the volume list to refresh after any mutation, so that the UI always reflects the machine's real state.
27. As a machine owner, I want meaningful error messages when the machine connection fails or podman rejects an operation, so that I can tell network problems from invalid input.
28. As a machine owner, I want to be protected from unbounded uploads, so that one oversized populate request cannot exhaust the backend server.
29. As a machine owner, I want volume operations to reuse the machine's existing SSH connection, so that operations are fast and do not open redundant tunnels.
30. As an admin, I want the upload size limit configurable via environment variable, so that I can tune it to my deployment's storage.
31. As a machine owner, I want unknown volume names to produce a clean not-found error, so that stale UI state is obvious rather than confusing.

## Implementation Decisions

- **New `volumes` domain package** mirroring the containers pattern: handler (HTTP/JSON, standard error envelope) → service (business logic) → podman bindings over the cached per-machine SSH connection from the existing ConnectionManager. Volumes are **not persisted** in Podland's database; the service proxies podman directly.
- **Podman bindings surface** (verified in the forked podman checkout): `Create`, `Inspect`, `List`, `Prune`, `Remove`, `Exists`, `Export(io.Writer)`, `Import(io.Reader)`. The tunnel engines are used exactly as `ContainerService` uses `tunnel.ContainerEngine`.
- **No rename.** Podman bindings expose no volume rename/clone; implementing it as a composite create→export→import→delete was considered and explicitly rejected (risk of half-migrated or duplicated volumes, impossible safely while in use). Dropped from scope.
- **Routes** under `/api/machines/{id}/volumes`, inside the existing machine-access subtree:
  - `GET /` — list volumes
  - `POST /` — create volume
  - `POST /prune` — prune unused volumes
  - `GET /{volumeName}` — inspect
  - `DELETE /{volumeName}?force=` — delete (force defaults to false)
  - `GET /{volumeName}/export` — stream tar download
  - `POST /{volumeName}/populate` — multipart upload
- **Permission mapping** (grilling ruling, reuses level-aware middleware from the secrets epic; no schema change):
  - **read** — list, inspect, export. Export is deliberately read-level: consistent with logs/inspect, users who can read a machine's state can read its data.
  - **write** — additionally create, delete, populate, prune.
- **Create** passes through the full podman `VolumeCreateOptions`: optional name (podman generates one for anonymous volumes when omitted), labels, driver, driver options. Request validated with struct tags + validator like other domains. Frontend form keeps name + labels visible and hides driver/options behind a collapsible "advanced" section.
- **Delete** exposes `force` (default false). Without force, podman refuses to remove in-use volumes and the backend maps that to a client error; with force, podman stops dependent containers first (its own semantics, including stop timeout). UI shows a danger confirmation with the force checkbox, mirroring the images/containers pattern.
- **Prune** is a write-level operation returning podman's prune reports. UI shows a danger confirmation. (Included by user decision despite multi-user blast radius; write-level gating is the mitigation.)
- **Export** streams the tar from podman's `Export` writer directly to the HTTP response — no server-side buffering, no size cap needed on the download path. `Content-Disposition` filename is `<volumeName>.tar`. Frontend downloads via authenticated fetch → blob → save.
- **Populate** accepts multipart form data with two shapes (user ruling: both):
  - multiple plain file parts, whose relative paths are preserved — the backend assembles them into a tar in a server temp file and imports it;
  - a single `.tar` part — streamed to import directly.
  - Semantics are podman-native **merge**: uploaded paths overwrite same-named files; other existing files are kept. No replace/clear mode (explicitly rejected).
- **Upload bound** by new environment variable `VOLUME_UPLOAD_MAX_BYTES` (default 2 GiB), enforced with `http.MaxBytesReader` while streaming parts to a temp file — the tar is never held in RAM. Exceeding the cap yields 413. Temp files are always cleaned up.
- **Frontend structure** follows the feature-folder convention: `features/volumes/` with an API client module, React Query hooks, and components. New "Volumes" tab on the machine detail page (after Containers). Per-row actions: Export, Populate, Delete; row click opens the inspect drawer. Create is a modal. Prune is a toolbar action. Mutations invalidate the volumes query cache. No SSE/live updates — plain refetch, consistent with containers.
- **Error handling** follows the existing envelope conventions: structured errors with requestId, clean 404 for unknown volume names, 403 for permission failures, connection failures surfaced as server errors. API contract documentation is added to the docs directory alongside the other resource families.
- **Anonymous volumes** are included in the list and distinguishable by a flag/type badge in the UI.

## Testing Decisions

- **Good tests assert external behavior only**: HTTP status codes, JSON response shapes, and for export/populate the actual tar bytes round-tripped through a real volume. No test inspects internal call sequences.
- **Seams (confirmed with user) — all existing, no new ones introduced:**
  1. `VolumeServiceInterface` (same shape as the containers service interface) lets handler unit tests mock the service via the existing shared test mock utilities.
  2. The existing `machines.ConnectionManagerInterface` seam covers service unit tests for input validation and connection-failure paths; real podman interactions are not unit-mocked.
  3. The existing integration-test harness (in-process server + real podman machine + seeded users at read/write levels) proves full round-trips: create → populate → list → inspect → export (verify tar contents) → delete → 404, prune behavior, force-delete of an in-use volume, and the read-vs-write permission matrix.
  4. The existing frontend MSW server covers hook and component tests, including upload and download flows (download verified via blob interception) and permission-gated UI states.
- **Prior art**: containers service/handler unit tests (connection-error and validation styles), secrets integration slices for the permission matrix, images UI tests for delete-with-force confirmation patterns.

## Out of Scope

- **Volume rename** — no podman bindings support; composite implementation rejected during grilling.
- **Replace/clear-contents populate mode** — merge only.
- **Volume duplicate/clone** — export + create + import covers it manually if ever needed.
- **Mounting volumes into containers at container-create time** — the existing unimplemented mount fields in the container create DTO remain untouched.
- **SSE/live updates** for volume events and auto-refreshing lists.
- **File browser / file-manager UI** inside a volume (browsing, single-file view/edit/delete).
- **Quadlet `.volume` unit generation** or any quadlet integration.
- **Scheduled exports/backups** or snapshotting.
- **Podland-side persistence** of volume metadata (never stored; podman is the source of truth).

## Further Notes

### Vertical Slices

Implementation should follow the repo's tracer-bullet slicing; suggested ordering:

1. **Backend volumes domain: list + inspect + create + delete (with force)** — service, handler, DTOs, routes, unit tests.
2. **Backend export endpoint** — streaming tar download.
3. **Backend populate endpoint** — multipart parsing, temp-file tar assembly, direct-tar path, `VOLUME_UPLOAD_MAX_BYTES` enforcement.
4. **Backend prune endpoint.**
5. **Backend integration tests** — round-trips, permission matrix, tar content verification.
6. **Frontend API client + hooks.**
7. **Frontend Volumes tab: list + inspect drawer.**
8. **Frontend create modal + delete/prune confirmations.**
9. **Frontend export download + populate upload modal.**

### Grilling rulings recap (2026-08-30)

- Rename: dropped (no podman support; composite too risky).
- Populate input: both plain files and ready tar.
- Populate semantics: merge only.
- Create: full podman option pass-through.
- Delete: force flag exposed.
- Export: download-only tar at read level.
- Prune: included.
- Upload cap: env-var `VOLUME_UPLOAD_MAX_BYTES`, default 2 GiB, streaming temp-file implementation.
- UI: Volumes tab + inspect drawer.
