---
title: Quadlets classify through podmanerr; conventions doc records the extraction
parent: podmanerr — one adapter owning podman's error model
date: 2026-09-07
status: ready-for-agent
labels:
  - backend
  - refactor
  - error-handling
  - quadlets
  - documentation
author: vibe-agent
---

## Parent

podmanerr — one adapter owning podman's error model (see `.scratch/240-podmanerr-adapter/issue.md`).

## What to build

The domain with the broadest heuristics migrates last, on top of the proven module:

- `isNotFoundError` → `podmanerr.IsNotFound`.
- `isAlreadyExistsError` → `podmanerr.IsAlreadyExists`.
- `isResourceRunningError` → `podmanerr.IsConflict` **or** a quadlet-local check for the `running` / `active` phrases — those two stay quadlet-private per the design session, because they read systemd-flavoured text that only quadlets interprets as resource-running; they are deliberately **not** in the shared vocabulary. Same `ErrQuadletResourceRunning` outcome, same `!force` guard.
- GetLogs and GetLogsWithStream stop re-copying the container phrases: `podmanerr.IsNotFound` → `ErrQuadletNotFound`.
- The quadlet classifier table tests are deleted with their functions; what remains pins sentinel mapping.

Explicitly untouched: `lifecycle.go` (systemctl exit-code/stderr classification stays quadlet-local — out of the epic's scope), and Import keeps re-synthesizing its plain `already exists` error rather than the sentinel.

Then the conventions doc edit: `docs/backend-conventions.md` names podmanerr as the third extracted reference in the duplication-across-domains ruling ("…the podman error model became `internal/podmanerr`"), plus one rule line: domains classify podman errors through `podmanerr`; phrase tables live only there; sentinels stay in `internal/errors`. Minimal, in the doc's existing references-accreted-from-extractions style — not a rewrite.

No doku API hunks — codes and statuses are unchanged.

## Acceptance criteria

- [ ] No podman phrase matching remains in the quadlets service except the quadlet-local `running`/`active` resource check
- [ ] `lifecycle.go` is unchanged
- [ ] Import's error shape is unchanged (still a plain error, not the sentinel)
- [ ] Conventions doc names podmanerr as the third extracted reference with the classification rule line
- [ ] `go test ./...` green; API behavior identical (404/409/400 codes unchanged)

## Blocked by

- `240-podmanerr-adapter-01-module-volumes-secrets`
