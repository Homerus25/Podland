---
title: Containers UI — list with state badges
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

Fill the machine detail page's Containers tab with a list of the machine's containers, showing name, image, and current state. This slice establishes the frontend containers module following the images-tab precedent: a per-feature service calling the shared API client against the existing backend container list endpoint, and a `useContainers(machineId)` React Query hook keyed under the per-machine hierarchy `['machines', machineId, 'containers']` so caches are isolated per machine.

State is communicated with color-coded badges (running green, exited gray, paused yellow, dead red, plus sensible fallbacks for created/restarting/unknown). The badge palette is defined once in shared UI utilities — not inside the containers feature — since later tabs reuse state vocabulary. The tab gets distinct loading, empty, and error states (error rendered via the shared error display with envelope code + message), replacing the current tab placeholder; the machine detail page's placeholder test is updated accordingly.

## Acceptance criteria

- [ ] Containers tab lists containers with name, image, and a state badge
- [ ] State badges are color-coded from a shared palette (running green, exited gray, paused yellow, dead red); unrecognized states still render a neutral badge
- [ ] `useContainers(machineId)` uses the query key hierarchy `['machines', machineId, 'containers']`
- [ ] Loading, empty, and error states are visually distinct; error shows the envelope code and message
- [ ] Containers tab placeholder is replaced and the machine detail page tests are updated to match
- [ ] Vitest + RTL + MSW tests cover: rendering rows from a mocked list response, one test per badge state, empty list, and error response

## Blocked by

None - can start immediately (the `130-images-ui` streaming helper this PRD depends on has landed).
