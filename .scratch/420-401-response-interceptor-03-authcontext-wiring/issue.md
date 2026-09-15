---
title: Session-death wiring in AuthContext
parent: 420-401-response-interceptor
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - testing
author: vibe-agent
---

## Parent

`.scratch/420-401-response-interceptor/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

When the session dies mid-app, the user is taken out through the one
existing exit path and lands on `/login` with their deep link preserved —
silently, no error screen. `AuthProvider` registers the policy's
session-expired seam on mount (unregisters on unmount) and the handler
runs the existing `logout()`: best-effort revoke, drop the token, drop the
user — plus a new query-cache clear, so a different user logging in next
never sees the previous user's cached lists. Manual logout gets the same
cache clear for free. Demo: authenticated app, a query 401s and refresh is
rejected → the login page renders with the original location attached, and
the cache is empty.

## Acceptance criteria

- [ ] `AuthProvider` registers a session-expired handler on mount and unregisters on unmount; the handler runs the existing `logout()`
- [ ] `logout()` clears the query cache via `useQueryClient().clear()` in its `finally`, alongside the existing local-state drop; `login()` does not clear
- [ ] App-level test: authenticated app on a page, a query 401s and the refresh is rejected mid-session → the user ends on `/login` with the requested location preserved in state
- [ ] `npx vitest run` is green; touched files pass `npx biome check`

## Blocked by

- `420-401-response-interceptor-02-interceptor-policy`

## Technical notes

- `logout()`'s `POST /auth/logout` rides the bare transport (best-effort revoke; harmless when the session is already dead).
- `AuthProvider` renders inside `QueryClientProvider`, so `useQueryClient()` reaches the app-wide instance; the test render helper keeps its own fresh `QueryClient`, which the clear assertion can observe.
- Stream coverage is parallel slice 04; this slice's seam wiring covers both transports since the seam is policy-owned.
