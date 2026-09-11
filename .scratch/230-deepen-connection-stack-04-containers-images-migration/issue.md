---
title: Containers and images migrate to WithConnection
parent: Deepen the connection stack — one ConnectionManager seam with real teardown
date: 2026-09-07
status: ready-for-agent
labels:
  - connections
  - containers
  - images
  - backend
  - refactor
author: vibe-agent
---

## Parent

Deepen the connection stack (see `.scratch/230-deepen-connection-stack/issue.md`).

## What to build

Copy the volumes reference slice (`.scratch/230-deepen-connection-stack-03-volumes-reference-migration`) across two domains:

- **Containers** (eight acquisition sites: list, inspect, create, start/stop, delete, logs) and **images** (four sites: list, delete, prune, pull) migrate their services to `WithConnection` with the closure-capture shape.
- Both domains keep their own error mapping (not-found, in-use, conflict) — only the acquisition prologue (`GetConnection` → map → `defer Release`) disappears.
- Streaming endpoints (container logs handler, image pull handler) keep their orchestration shape; only the **service-level** acquisition crosses the new seam. Streaming consolidation is explicitly out of scope for this epic.
- Delete each domain's "connection error → ErrConnectionFailed" service-test pins, exactly as the volumes slice did. Handler contract tests and integration tests untouched; no doku hunks.

## Acceptance criteria

- [ ] No `GetConnection`/`Release` references remain in the containers or images domains
- [ ] Both domains' acquisition failures now carry the cause log via the seam (these domains already logged it; behavior preserved, now guaranteed)
- [ ] All containers/images unit tests, handler contract tests, and `go test ./...` green
- [ ] API behavior identical; streaming endpoints behave as before

## Blocked by

- `230-deepen-connection-stack-03-volumes-reference-migration` (copies its pattern)
