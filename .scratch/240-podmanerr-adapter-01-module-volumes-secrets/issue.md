---
title: podmanerr module + volumes and secrets migrate (reference slice)
parent: podmanerr — one adapter owning podman's error model
date: 2026-09-07
status: ready-for-agent
labels:
  - backend
  - refactor
  - error-handling
  - volumes
  - secrets
author: vibe-agent
---

## Parent

podmanerr — one adapter owning podman's error model (see `.scratch/240-podmanerr-adapter/issue.md`).

## What to build

Create `internal/podmanerr` with the five-function surface settled in the design session — `IsNotFound`, `IsAlreadyExists`, `IsConflict`, `IsClientError`, `Message` (the exact vocabulary table and semantics are in the parent epic; keep them verbatim). The module ships complete: full typed-and-phrase vocabulary from day one, importing only stdlib and podman's `errorhandling`.

One table test in the module ports **every** existing classification pin from the per-domain tests — the volumes classifier tables, the quadlets classifier tables (including the negative pins like bare `"file exists"` not being already-exists), the secrets HTTP-stub bodies (404 body → not-found; cause `"secret name in use"` → already-exists; `"is in use by container"` → conflict), and the images phrases — deduped into one corpus. Each unification delta gets its own row (containers-style `not found` phrases classify not-found; contains-style cause matching; images conflict superset).

Then migrate the two typed-first domains and delete their local classifiers:

- **volumes**: `mapInUseError` → `IsConflict` → `ErrVolumeInUse.WithMessage(podmanerr.Message(err))`; `mapCreateError` → `IsClientError` → `ErrInvalidRequest.WithMessage(...)`; `isVolumeNotFound` → `IsNotFound`. The three locals and their `errors.As` digging are deleted.
- **secrets**: `isPodmanNotFound` → `IsNotFound`; `isPodmanSecretNameInUse` → `IsAlreadyExists`; `isPodmanSecretInUse` → `IsConflict`; `podmanErrorMessage` → `podmanerr.Message`. The four locals are deleted.

Per-domain classifier table tests are deleted with their functions; what remains pins sentinel mapping only. Handler contract tests are untouched — the envelope is identical, so no doku API hunks.

- This slice is the **reference implementation** the sweep tickets copy: it shows the call-site shape (`if podmanerr.IsNotFound(err) { return appErrors.ErrSecretNotFound }`) and how the corpus test replaces per-domain phrase pins.

## Acceptance criteria

- [ ] `internal/podmanerr` exists with the five-function surface and imports nothing from `internal/errors`
- [ ] The module's table test ports every existing classification pin, negatives included, plus one row per unification delta
- [ ] No local podman-error classifier remains in volumes or secrets (no `errors.As` into `ErrorModel`, no podman phrase matching)
- [ ] All volumes/secrets unit and handler tests, and `go test ./...`, green
- [ ] API behavior identical: same codes and statuses; secrets' carried podman message text unchanged where pinned

## Blocked by

None (can start immediately).
