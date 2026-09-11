---
title: Quadlets UI — delete with stop-first option
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

Add a delete flow to the Quadlets tab, mirroring the images/containers delete precedent: a delete action per list row opens a confirmation modal naming the quadlet. The modal exposes a stop-first option (default on, matching the backend contract's default) that stops the unit before the quadlet file is removed, and a force option (default off) for aggressive teardown, both mapped onto the delete endpoint's `stop` and `force` query parameters.

Confirming sends the delete with the chosen options; on success the modal closes and the quadlet list refetches so the unit disappears. Failures (quadlet not found, ssh failure, systemctl errors) surface with the envelope code + message and leave the modal open so the user can retry or cancel.

## Acceptance criteria

- [ ] A delete action on each quadlet row opens a confirmation modal naming the quadlet
- [ ] Stop-first option defaults to on and maps to the `stop` query parameter; force defaults to off and maps to `force`
- [ ] Confirming sends the delete request; canceling closes the modal without a request
- [ ] Quadlet list refetches after a successful delete
- [ ] Backend errors surface the envelope code and message and keep the modal open
- [ ] Vitest + RTL + MSW tests cover: confirmation before request, query parameters reflecting the chosen options, list refetch on success, and error envelope display

## Blocked by

- `.scratch/150-quadlets-ui-01-list/issue.md`
