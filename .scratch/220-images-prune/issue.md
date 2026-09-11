---
title: Image prune — backend endpoint + Images tab delete-unused button
parent: none
date: 2026-09-04
status: ready-for-agent
labels:
  - images
  - feature
  - podman-integration
author: vibe-agent
---

## Problem Statement

The Images tab only offers per-image delete. Cleaning up a machine accumulated with unused images means deleting them one by one. Podman already has a bulk verb for this — image prune — but Podland neither exposes it via the API nor surfaces it in the UI.

## Solution

One write-gated backend endpoint, `POST /api/machines/{id}/images/prune`, that removes **all unused images** on the machine in one call (podman `image prune -a` semantics: every image not used by any container, tagged or not — plain dangling-only prune is NOT the requested behavior) and returns podman's per-image reports. One frontend slice adds a "Delete unused images" toolbar action to the Images tab behind a danger confirmation, with a post-prune report, mirroring the volume prune UX 1:1.

## User Stories

1. As a user with write access, I want one action that removes every image no container uses, so that reclaiming disk space is not a per-image chore.
2. As a user with write access, I want a report of what was removed and how much was reclaimed, so that bulk cleanup is transparent.
3. As a user, I want images in use by containers to survive the operation, so that running workloads are never broken.
4. As a user with read access, I want the bulk-delete control hidden, so that I never start an action I cannot finish.

## Out of Scope

- Dangling-only prune mode or prune filters (label, until) — can follow later if wanted
- Build-cache pruning

## Slices

1. `220-images-prune-01-backend-prune-endpoint` — prune endpoint + reports — no blockers
2. `220-images-prune-02-frontend-prune-button` — Images tab button + confirmation + report — blocked by 220-01
