---
title: Quadlets and secrets migrate to WithConnection (drift heals)
parent: Deepen the connection stack — one ConnectionManager seam with real teardown
date: 2026-09-07
status: ready-for-agent
labels:
  - connections
  - quadlets
  - secrets
  - backend
  - refactor
author: vibe-agent
---

## Parent

Deepen the connection stack (see `.scratch/230-deepen-connection-stack/issue.md`).

## What to build

Copy the volumes reference slice (`.scratch/230-deepen-connection-stack-03-volumes-reference-migration`) across the two remaining non-machines domains:

- **Quadlets** (ten acquisition sites: CRUD, generate, import, auto-update, lifecycle including the SSH-exec lifecycle path) and **secrets** (four sites: list, inspect, create, delete) migrate to `WithConnection`.
- These are the domains whose 14 call sites today **silently drop the cause log** required by `docs/backend-conventions.md` — after this slice the log is structurally guaranteed by the seam, and the drift cannot regress.
- Domain-specific mapping (quadlet not-found heuristics, secret in-use conflicts) stays put; only the acquisition prologue disappears.
- Delete each domain's connection-error service-test pins, as in the volumes slice. Handler contract tests and integration tests untouched; no doku hunks.

## Acceptance criteria

- [ ] No `GetConnection`/`Release` references remain in the quadlets or secrets domains
- [ ] Acquisition failures in both domains log op/machineID/userID/cause via the seam
- [ ] All quadlets/secrets unit tests, handler contract tests, and `go test ./...` green
- [ ] API behavior identical

## Blocked by

- `230-deepen-connection-stack-03-volumes-reference-migration` (copies its pattern)
