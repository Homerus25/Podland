---
title: Containers UI — create container form
parent: 140-containers-ui
date: 2026-08-26
status: open
labels:
  - ready-for-agent
  - frontend
  - containers
  - feature
author: vibe-agent
---

## Parent

`.scratch/140-containers-ui/issue.md`

## What to build

Add a "create container" action to the Containers tab that opens a modal form built with react-hook-form (same pattern as the machine form). Fields: image (required), name (optional), command (optional). The form maps onto the backend's container create DTO.

Client-side validation prevents an empty image from ever reaching the server, showing a validation message instead. Submitting sends the create request; on success the modal closes and the containers query is invalidated so the new container appears without a manual reload. On failure the modal stays open and the error envelope (code + message) is surfaced so causes like a nonexistent image are visible.

## Acceptance criteria

- [ ] Create action opens a modal form with required image and optional name and command fields
- [ ] Submitting with an empty image is blocked client-side with a validation message; no request is sent
- [ ] A valid submit sends the create request with the payload matching the backend create DTO shape
- [ ] On success the modal closes and the containers query is invalidated so the new container appears
- [ ] On failure the modal stays open and the error envelope's code and message are shown
- [ ] Vitest + RTL + MSW tests cover: validation of empty image, submit payload shape, success close + invalidation, and error envelope rendering

## Blocked by

- `.scratch/140-containers-ui-01-list/issue.md`
