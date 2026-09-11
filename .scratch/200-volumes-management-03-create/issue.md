---
title: Volume create endpoint (POST /api/machines/{id}/volumes)
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

`POST /api/machines/{id}/volumes` with a JSON body passing through the full podman create surface: optional `name` (podman generates one when omitted, yielding an anonymous volume), `labels` (string map), `driver` (defaults to podman's `local`), and `options` (string map of driver options). Returns 201 with the created volume's config, including the generated name for anonymous volumes — callers must be able to learn that name.

Write-level permission: read-level users get 403. Invalid payloads (wrong types, malformed body) get the standard validation error responses. Name-format validation is podman's job; pass through its errors as client errors.

## Acceptance criteria

- [ ] `POST /api/machines/{id}/volumes` with a name returns 201 and the created volume's config
- [ ] Omitting the name creates an anonymous volume; the generated name is present in the response
- [ ] Labels, driver, and driver options are honored (verified by inspecting the created volume)
- [ ] Read-level users get 403; users without access get 403; unknown machine gets 404
- [ ] Malformed request bodies produce the standard validation error response
- [ ] Podman-rejected names/errors map to client errors, not 5xx
- [ ] Unit tests: handler validation and service mapping (including the anonymous path); integration test: named create, anonymous create, label/driver-option round-trip via inspect, read-user 403

## Blocked by

- 200-volumes-management-01-list (volumes domain skeleton and route family)
