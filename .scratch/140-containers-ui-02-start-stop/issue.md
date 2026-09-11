---
title: Containers UI — start and stop with pending states
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

Add per-row lifecycle buttons to the container list: start appears on stopped containers, stop on running ones. Both are React Query mutations that call the existing backend start/stop endpoints for that container.

While a mutation runs, only the affected row's buttons show pending state and are disabled (no double-clicks); other rows stay fully usable. On success the containers query is invalidated so the list refetches and shows the trustworthy new state — no optimistic updates. On failure the pending state clears and the backend's error envelope (code + message) is surfaced so causes like a missing image are diagnosable.

## Acceptance criteria

- [ ] Start button renders for stopped containers, stop button for running containers
- [ ] Clicking start/stop sends the request to the correct lifecycle endpoint for that container
- [ ] The clicked row's buttons show pending state and are disabled while the operation runs; other rows are unaffected
- [ ] On success, the containers query is invalidated and the list reflects the new state
- [ ] On failure, the error envelope's code and message are shown and pending state clears
- [ ] Vitest + RTL + MSW tests cover: correct URL/method per action, per-row pending state, invalidation on success, and error envelope rendering on failure

## Blocked by

- `.scratch/140-containers-ui-01-list/issue.md`
