---
title: Quadlets UI — list with resource type and state badges
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

Fill the machine detail page's Quadlets tab with a list of the machine's quadlets, showing name, resource type (container, pod, image, volume, network), and current state. This slice establishes the frontend quadlets module following the containers-tab precedent: a per-feature service calling the shared API client against the existing backend quadlet list endpoint, and a `useQuadlets(machineId)` React Query hook keyed under the per-machine hierarchy `['machines', machineId, 'quadlets']` so caches are isolated per machine.

State is communicated with color-coded badges: running green, stopped gray, error red, unknown neutral. The badge palette is shared with containers; this slice extends it with the quadlet vocabulary (`stopped`, `error`) rather than defining a second palette — `running` already maps to green and unrecognized values already fall back to neutral. The tab gets distinct loading, empty, and error states (error rendered via the shared error display with envelope code + message), replacing the current tab placeholder; the machine detail page's placeholder test is updated accordingly.

## Acceptance criteria

- [ ] Quadlets tab lists quadlets with name, resource type, and a state badge
- [ ] State badges are color-coded from the shared palette (running green, stopped gray, error red, unknown neutral)
- [ ] `useQuadlets(machineId)` uses the query key hierarchy `['machines', machineId, 'quadlets']`
- [ ] Loading, empty, and error states are visually distinct; error shows the envelope code and message
- [ ] Quadlets tab placeholder is replaced and the machine detail page tests are updated to match
- [ ] Vitest + RTL + MSW tests cover: rendering rows from a mocked list response (including each resource type), one test per badge state, empty list, and error response

## Blocked by

None - can start immediately (the `140-containers-ui` shared log viewer and streaming helper this PRD depends on have landed).
