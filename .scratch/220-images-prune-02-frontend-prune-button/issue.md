---
title: Images tab "Delete unused images" button with confirmation and report
parent: Image prune — backend endpoint + Images tab delete-unused button
date: 2026-09-04
status: ready-for-agent
labels:
  - images
  - frontend
  - feature
author: vibe-agent
---

## Parent

Image prune (`.scratch/220-images-prune/issue.md`); backend contract in `.scratch/220-images-prune-01-backend-prune-endpoint/issue.md`.

## What to build

A "Delete unused images" toolbar action in the Images tab (write-level users only), placed with the pull form. A danger confirmation precedes the call and spells out the blast radius: every image not used by a container on this machine will be removed; images backing running containers are safe. After confirmation, the result renders as a report listing each removed image with its reclaimed size (and any per-image errors), and the images query is invalidated.

Follow the volume prune UX 1:1 (toolbar action, confirmation, report view). The images list currently has no permission prop — thread the effective permission into it the same way the volumes tab does, and hide the button for read-level users.

## Acceptance criteria

- [ ] Button visible only to write-level users
- [ ] Danger confirmation precedes the call, stating that all unused images are removed
- [ ] Result view lists removed images with reclaimed sizes, and any per-image errors
- [ ] Images list refreshes after prune
- [ ] MSW tests: confirmation gating, report rendering, invalidation, hidden button for read users, error display

## Blocked by

- 220-images-prune-01-backend-prune-endpoint (prune endpoint)
