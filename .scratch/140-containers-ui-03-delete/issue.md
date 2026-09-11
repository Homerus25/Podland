---
title: Containers UI — delete with confirmation
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

Add a per-row delete action to the container list, guarded by the shared confirmation modal so a container is never removed by accident. No network request happens until the user explicitly confirms; cancelling closes the modal without side effects.

Confirming sends the delete request for that container with pending state on the confirm action, and on success the containers query is invalidated so the row disappears from the refreshed list. On failure the error envelope (code + message) is surfaced. Force-delete is out of scope for this slice.

## Acceptance criteria

- [ ] Delete button opens the shared confirmation modal; no request is sent before explicit confirmation
- [ ] Cancelling the modal sends no request and leaves the list untouched
- [ ] Confirming sends the delete request for the targeted container, with pending state on the confirm action
- [ ] On success, the containers query is invalidated and the container disappears from the list
- [ ] On failure, the error envelope's code and message are shown
- [ ] Vitest + RTL + MSW tests cover: no request on cancel, correct request on confirm, invalidation on success, and error envelope rendering on failure

## Blocked by

- `.scratch/140-containers-ui-01-list/issue.md`
