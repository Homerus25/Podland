---
title: Quadlets UI — start, stop, and restart with exposed options
parent: 150-quadlets-ui
date: 2026-08-26
status: open
labels:
  - ready-for-agent
  - frontend
  - quadlets
  - feature
author: vibe-agent
---

## Parent

`.scratch/150-quadlets-ui/issue.md`

## What to build

Give each quadlet row start, stop, and restart actions that call the corresponding lifecycle endpoints over the shared API client, as React Query mutations in the quadlets module. While a mutation is in flight its button (and the row's other lifecycle actions) show a pending/disabled state so slow systemd operations don't invite double clicks. On success the quadlet list is invalidated so the row's state badge reflects what the machine now reports; backend failures surface with the envelope code + message.

Per the accepted breakdown, stop and restart expose their options in the UI rather than hiding defaults: a force toggle (default off, mapping to the endpoint's `force` query parameter) and a timeout input in seconds (default unset, omitted from the request when empty, mapping to `timeout`). Start takes no options per the backend contract.

## Acceptance criteria

- [ ] Start, stop, and restart actions on each quadlet row hit the correct lifecycle endpoints
- [ ] Lifecycle buttons show pending state and resist double activation while in flight
- [ ] Stop and restart expose force (default off) and timeout (default unset) mapped onto the `force` and `timeout` query parameters; an unset timeout sends no parameter
- [ ] Start sends no options
- [ ] A successful action invalidates the quadlet list so the state badge updates
- [ ] Backend errors surface the envelope code and message
- [ ] Vitest + RTL + MSW tests cover: correct URLs and query params per action, pending states, list invalidation on success, and error envelope display

## Blocked by

- `.scratch/150-quadlets-ui-01-list/issue.md`
