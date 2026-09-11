---
title: Volume populate endpoint (POST /api/machines/{id}/volumes/{volumeName}/populate)
parent: Volume management for machines (list, inspect, create, delete, prune, export, populate)
date: 2026-08-30
status: ready-for-agent
labels:
  - volumes
  - backend
  - feature
  - podman-integration
author: vibe-agent
---

## Parent

Volume management for machines (see `.scratch/200-volumes-management/200-0-volumes-management-master.md` for full rulings).

## What to build

`POST /api/machines/{id}/volumes/{volumeName}/populate` accepting `multipart/form-data` in exactly one of two shapes (grilling ruling: support both):

- **files shape**: one or more `files` file parts; the part filenames carry relative paths which must be preserved. The backend assembles a tar in a server temp file and imports it.
- **archive shape**: exactly one `archive` part containing a ready-made tar, streamed to a temp file and imported as-is.

Sending both shapes, or neither, is a 400. Semantics are podman-native **merge**: uploaded paths overwrite same-named files; everything else in the volume is kept. This must be documented in the API contract — there is no replace mode.

Uploads are bounded by a new environment variable `VOLUME_UPLOAD_MAX_BYTES` (default 2 GiB), enforced while streaming parts to disk via the standard max-bytes reader — nothing is ever buffered in RAM. Exceeding the cap yields 413 with a clear error code. Temp files are removed on every path, including failures. Write-level permission; unknown volume 404.

## Acceptance criteria

- [ ] Files shape: uploaded files land in the volume with their relative paths preserved
- [ ] Archive shape: an uploaded tar imports with its structure intact
- [ ] Merge semantics: pre-existing files at other paths survive a populate (verified end-to-end)
- [ ] Both shapes at once, or an empty upload, return 400
- [ ] Uploads exceeding `VOLUME_UPLOAD_MAX_BYTES` return 413 with a distinct error code; the default is 2 GiB and the env override is honored
- [ ] Parts are streamed to temp files (no RAM buffering); temp files are cleaned up on success and failure
- [ ] Read-level users get 403; unknown volume name returns 404
- [ ] Unit tests: multipart shape validation, cap enforcement, env parsing; integration test against real podman: populate via files shape and archive shape, then verify contents via the bindings' export (untar and compare), including the merge case

## Blocked by

- 200-volumes-management-01-list (volumes domain skeleton and route family)
- Soft dependencies: 200-volumes-management-03-create and 200-volumes-management-06-export make test fixtures and verification more convenient
