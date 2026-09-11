---
title: Volume export endpoint (GET /api/machines/{id}/volumes/{volumeName}/export)
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

`GET /api/machines/{id}/volumes/{volumeName}/export` streaming the volume's contents as a tar archive download. The podman bindings expose export as a write-to-`io.Writer` call; the handler wires that writer directly to the HTTP response — no server-side buffering and no size cap on the download path.

Headers: `Content-Type: application/x-tar`, `Content-Disposition: attachment; filename="<volumeName>.tar"`. Read-level permission (grilling ruling: users who can read a machine's state can read its data). Unknown volume names get the standard 404.

## Acceptance criteria

- [ ] `GET .../{volumeName}/export` streams a valid tar archive of the volume contents
- [ ] Content-Type is `application/x-tar`; Content-Disposition carries `<volumeName>.tar`
- [ ] Export works for read-level users; no-access users get 403
- [ ] Unknown volume name returns 404
- [ ] Unit tests: handler streaming and header behavior via the mocked service seam
- [ ] Integration test against real podman: seed a volume with known files (via the bindings' import), export, untar the response body, verify file contents match

## Blocked by

- 200-volumes-management-01-list (volumes domain skeleton and route family)
