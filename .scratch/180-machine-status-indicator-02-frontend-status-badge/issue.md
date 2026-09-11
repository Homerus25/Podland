---
title: "Frontend: status badge on machine cards"
parent: 180-machine-status-indicator
date: 2026-08-29
status: ready-for-agent
labels:
  - ready-for-agent
  - machines
  - frontend
  - feature
author: vibe-agent
---

## Parent

`.scratch/180-machine-status-indicator/issue.md`

## What to build

A color-coded status indicator on every machine card of the overview page, fed by the live status endpoint. Each card runs its own status query through the shared API client, keyed per machine, with stale time zero — every mount of the overview triggers a fresh probe. No polling, no refetch interval: status refreshes only when the page is (re)opened, keeping the app's no-polling precedent intact. Cards render immediately from the machines list; status fills in per card, independently, so the list is never blocked by slow or dead hosts.

The indicator is a compact Chakra Badge with a colored dot plus a text label (colorblind-safe), placed in the card header to the left of the permission badge. States:

- green "online" — endpoint answered online
- red "offline" — endpoint answered offline
- gray "checking" — request in flight
- gray "unknown" — the status request itself failed (the card communicates uncertainty instead of guessing)

The `checkedAt` timestamp from the response is surfaced as a tooltip on the badge so the user can judge freshness.

## Acceptance criteria

- [ ] Every machine card shows a status badge in its header, left of the permission badge
- [ ] Green "online" and red "offline" render from the endpoint's response, each card queried individually
- [ ] Gray "checking" shows while the request is in flight; gray "unknown" shows if the request errors
- [ ] The checkedAt timestamp appears as a tooltip on the badge
- [ ] Text labels are always present alongside the colors (colorblind-safe)
- [ ] Re-opening the overview re-probes every machine (stale time zero); no polling or refetch interval is introduced anywhere
- [ ] The machines list renders without waiting for any status response; indicators fill in independently
- [ ] MSW tests: online/offline/checking/unknown render correctly, tooltip present, one status request fired per card machine id
- [ ] The shared status badge gets a stable test id and its own component tests, following the state badge pattern

## Blocked by

- 180-machine-status-indicator-00-prefactor-machine-card
- 180-machine-status-indicator-01-backend-status-endpoint
