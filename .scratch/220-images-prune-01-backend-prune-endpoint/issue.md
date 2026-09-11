---
title: Image prune endpoint (POST /api/machines/{id}/images/prune)
parent: Image prune — backend endpoint + Images tab delete-unused button
date: 2026-09-04
status: ready-for-agent
labels:
  - images
  - backend
  - feature
  - podman-integration
author: vibe-agent
---

## Parent

Image prune (see `.scratch/220-images-prune/issue.md`).

## What to build

`POST /api/machines/{id}/images/prune` removing all unused images on the machine in one call and returning 200 with podman's prune reports: one entry per removed image with its ID and reclaimed size (and any per-image error podman reports). Images in use by containers are untouched.

The podman call is the tunnel image engine's prune (`ImageEngine.Prune`, the same tunnel engine the image service already uses for list/delete/pull) invoked with `All: true` so **every** unused image is removed — tagged or not, matching `podman image prune -a`. Without `All`, podman only removes dangling (untagged) images, which is not the requested behavior.

Write-level permission — bulk deletion is a wide blast radius on a shared machine; this matches the volume prune ruling.

Routing note: unlike the volumes route family, there is no shadowing hazard — the images subtree's only parametric route is a `DELETE`, so a literal `POST /prune` cannot be swallowed.

## Acceptance criteria

- [ ] `POST /api/machines/{id}/images/prune` returns 200 with a report entry per removed image (id + reclaimed size + per-image error when present), same response shape as the volume prune reports
- [ ] All unused images are gone afterwards; images used by containers survive
- [ ] Read-level users get 403
- [ ] Unit tests for report mapping and error handling; integration test: pull at least one image, prune, verify report entries and disappearance via the list endpoint
- [ ] Endpoint documented in `doku/description_images_api.md`

## Blocked by

None (can start immediately).
