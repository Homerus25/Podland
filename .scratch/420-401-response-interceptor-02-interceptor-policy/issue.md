---
title: The 401 policy and the response interceptor
parent: 420-401-response-interceptor
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - refactoring
  - testing
author: vibe-agent
---

## Parent

`.scratch/420-401-response-interceptor/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

Mid-session token expiry stops being a per-widget error with a doomed
retry. A response interceptor on the shared axios instance owns 401s: it
runs one single-flight refresh on the bare transport; on success every
affected request replays once with the new token and the app keeps working
as if nothing happened. On session death — refresh rejected (auth or
network), or a replay that 401s again — a registered session-expired seam
fires exactly once and the caller receives the original 401 error. React
Query stops retrying deterministic 4xx. The loop is pinned by MSW stories
in the shared suite; the MachinesPage story that pinned the old per-widget
behavior is deleted. Demo: a query whose first response is 401 and whose
replay succeeds resolves normally, with exactly one refresh call on the
wire.

## Acceptance criteria

- [ ] `shared/utils/api` exposes `handleUnauthorized(): Promise<boolean>` — resolves `true` only when a fresh token was installed; `false` means the session is dead and the seam has already fired
- [ ] Single-flight: N concurrent 401s trigger exactly one `POST /auth/refresh` (shared in-flight promise via the bare transport from slice 01); all queued callers replay with the new token
- [ ] The response interceptor replays a 401ing request once with the new token; the replay is marked (request config) so a second 401 goes straight to session death without refreshing again
- [ ] Session-expired seam: register/unregister API in `shared/utils/api`; firing runs through an idempotent once-latch that resets whenever a new token is installed (`setBearerToken` with a non-null token); firing with no handler registered is a no-op
- [ ] Any refresh failure — auth rejection or network error — is session death; the caller's promise rejects with the original 401 error after the seam fires
- [ ] `main.tsx` retry predicate: retry once only on network errors or 5xx; never on any 4xx
- [ ] MSW stories in the `shared/utils/api` suite: 401 → refresh → replay succeeds; refresh rejected → seam fired exactly once and caller gets the original 401; concurrent 401s → exactly one refresh; login 401 on the bare transport → no refresh attempt, no seam; replayed request 401s again → seam once, no second refresh
- [ ] The MachinesPage per-widget 401-alert story is deleted, with no per-feature replacement
- [ ] `npx vitest run` is green; touched files pass `npx biome check`

## Blocked by

- `420-401-response-interceptor-01-bare-auth-transport`

## Technical notes

- With slice 01 landed, no `/auth/*` call rides the intercepted instance, so the policy never meets its own refresh (or a login-failure 401) on the wire.
- Existing suites whose 401 handlers now trigger a refresh POST may hit MSW unhandled-request noise: give those tests an explicit refresh handler rather than loosening the policy.
- Streams and `AuthContext` wiring are deliberately out of scope here — slices 03 and 04.
