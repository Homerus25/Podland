---
title: Containers and images classify through podmanerr
parent: podmanerr — one adapter owning podman's error model
date: 2026-09-07
status: ready-for-agent
labels:
  - backend
  - refactor
  - error-handling
  - containers
  - images
author: vibe-agent
---

## Parent

podmanerr — one adapter owning podman's error model (see `.scratch/240-podmanerr-adapter/issue.md`).

## What to build

Both string-only domains replace their inline `strings.Contains` phrase matching with the adapter, following the reference slice's call-site shape:

- **containers**: Delete's two error paths (the top-level engine error and the remove-report error), GetLogs, and GetLogsWithStream classify via `podmanerr.IsNotFound` → `ErrContainerNotFound`. The `ctx.Canceled` → `ErrOperationCancelled` branch stays as-is.
- **images**: Delete's per-image error classifies via `IsNotFound` → `ErrImageNotFound` and `IsConflict` → `ErrImageUsedByContainers` (**stays 400** — the status divergence is a watchlist item, not this ticket). Pull keeps its existing behavior with no new classification.

Deltas landing here (each already pinned in the module's corpus test): not-found matching broadens from the copied container/image phrases to the shared `no such` / `not found` vocabulary — it only bites on paths that already classify, and the previously untested containers not-found path is now covered by the shared table test. Images' conflict set becomes a superset (typed 409, `in use`, `used by`, `unable to delete`).

No doku API hunks — codes and statuses are unchanged.

## Acceptance criteria

- [ ] No inline podman phrase matching remains in the containers or images services
- [ ] `ErrImageUsedByContainers` responses are still 400; all handler contract tests green unchanged
- [ ] `go test ./...` green
- [ ] The image pull path is untouched (no new classification)

## Blocked by

- `240-podmanerr-adapter-01-module-volumes-secrets`
