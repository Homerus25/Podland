---
title: Bare transport for auth calls
parent: 420-401-response-interceptor
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - refactoring
author: vibe-agent
---

## Parent

`.scratch/420-401-response-interceptor/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The auth calls get their own transport so the future 401 policy can exist
without recursing. `shared/utils/api` creates and exports an
interceptor-free axios instance sharing the configured base URL, and
`authApi`'s login, refresh, and logout move onto it. For a user nothing
changes — same requests, same responses, all suites green. The point is
making the next slice easy: auth calls become structurally outside any 401
policy before that policy exists, and module flow stays one-directional
(`authApi` imports the transport module, never the reverse).

## Acceptance criteria

- [ ] `shared/utils/api` exports an unauthenticated axios instance: same configured base URL as the shared instance, no request or response interceptors attached to it
- [ ] `authApi`'s login, refresh, and logout all use the bare instance; `authApi` no longer uses the intercepted shared instance
- [ ] Auth calls carry no Authorization header (no request interceptor applies to them)
- [ ] `npx vitest run` is green — zero behavior change, existing MSW suites unchanged
- [ ] Touched files pass `npx biome check`

## Blocked by

- None (can start immediately)

## Technical notes

- The backend keeps `/auth/login`, `/auth/refresh`, and `/auth/logout` outside the JWT middleware — refresh and logout are cookie-based — so auth calls never need the bearer. Login failure is itself a 401 `INVALID_CREDENTIALS`, which is exactly the response that must never be interpreted as session death; the bare transport guarantees it can't be.
- Do not add the response interceptor, the policy, or any seam here; they ride in `420-401-response-interceptor-02`.
