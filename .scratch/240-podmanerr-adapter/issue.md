---
title: podmanerr — one adapter owning podman's error model
parent: none
date: 2026-09-07
status: ready-for-agent
labels:
  - backend
  - refactor
  - error-handling
author: vibe-agent
---

## Problem Statement

Classifying podman failures is an adapter responsibility re-implemented per domain in three divergent mechanisms, so a podman upgrade forks behaviour domain by domain:

- **Typed digging** (volumes, secrets): `errors.As` into podman's `ErrorModel`, switch on `ResponseCode`, with string fallbacks.
- **Inline phrase matching** (containers, images): `strings.Contains` on copied literal podman messages.
- **Heuristic predicates** (quadlets): package-private `isNotFoundError` / `isAlreadyExistsError` / `isResourceRunningError` matching broad substrings, plus the container phrases re-copied for the logs endpoints.

A string fallback is forced, not a style choice: the podman tunnel layer destroys type info on several paths (volume/secret inspect 404s arrive as plain `fmt.Errorf` strings, image-remove per-item errors are `errors.New` from response strings). So any adapter must be typed-first **and** phrase-aware.

`docs/backend-conventions.md` blesses sibling copies only up to the third domain — five have copied. This extraction is what the doc itself prescribes at this count, the same way the endpoint ritual became `internal/web.Handle` and the handler-test scaffolding became `testutils.Perform`.

The design below was settled in an architecture grilling session on 2026-09-07; the inline surface is that session's decision, not a suggestion.

## Solution

One leaf module, `internal/podmanerr`, owns podman's error model. Surface (keep exactly):

| Function | Typed (`ErrorModel.ResponseCode`) | Phrases (lowercase substring) | Cause field |
|---|---|---|---|
| `IsNotFound` | 404 | `no such`, `not found` | — |
| `IsAlreadyExists` | — | `already exists` | contains `name in use` |
| `IsConflict` | 409 | `in use`, `used by`, `unable to delete` | — |
| `IsClientError` | 400–499 excl. 404/409 | `invalid argument`, `names must match` | — |
| `Message` | message with cause appended when non-empty, else `err.Error()` | | |

Semantics settled in the session:

- Typed check first, then the phrase table, matching lowercased.
- Predicates are **independent facts, not a partition** — a 409 saying "already exists" is both `IsConflict` and `IsAlreadyExists`. Call sites check most-specific first (`IsAlreadyExists` before `IsConflict`).
- Every phrase in the table is already matched somewhere in the codebase today; nothing invented, only unified. The unification deltas (containers' not-found matching broadens, secrets' exact cause compare becomes contains, images' conflict set becomes a superset) are each pinned by a row in the module's table test.
- `podmanerr` imports stdlib and podman's `errorhandling` only — never `internal/errors`. Sentinels stay in `internal/errors`, mapped to at domain call sites, so "sentinels stay domain-owned" is enforced by the import graph.

Migration is three slices in parallel after the first: module + volumes & secrets (typed-first, cleanest — the reference), then containers & images and quadlets + conventions doc.

## Out of Scope

- **systemctl/stderr classification** (`quadlets/lifecycle.go`) — different transport (plain SSH, exit codes + stderr, not the podman API) with its own vocabulary. Stays quadlet-local; earns its own adapter if a second consumer appears.
- **502 `CONNECTION_FAILED` mapping** — stays at the connection seam per the backend conventions; services never map acquisition errors.
- **`ctx.Canceled` → 504** — stays per call site; it is Go context semantics, not podman's error model.
- **Status normalization** — `ErrImageUsedByContainers` stays **400** while `VOLUME_IN_USE`/`SECRET_IN_USE` are 409. Watchlist item; contract changes travel alone.
- **Quadlets Import's plain `already exists` error** — it re-synthesizes a plain error instead of the sentinel today and keeps doing so; no contract change here.
- **Streaming `AppError`→event mapping and the per-domain engine seams** — separate candidates from the same review; `podmanerr` is a leaf over error values and composes with both.

## Slices

1. `240-podmanerr-adapter-01-module-volumes-secrets` — the module with the full vocabulary and table test; volumes & secrets migrate and delete their local classifiers — no blockers
2. `240-podmanerr-adapter-02-containers-images` — containers & images replace their inline phrase matching — blocked by 240-01
3. `240-podmanerr-adapter-03-quadlets-conventions` — quadlets' predicates and copied log phrases migrate; conventions doc names podmanerr as the third extracted reference — blocked by 240-01
