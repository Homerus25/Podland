---
title: Stream transport joins the 401 policy
parent: 420-401-response-interceptor
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - streaming
  - testing
author: vibe-agent
---

## Parent

`.scratch/420-401-response-interceptor/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

A mid-session expiry during an NDJSON operation (prune, auto-update,
logs) gets the same treatment as queries instead of dying as a per-widget
error. `streamNdjson` runs the shared policy when a response comes back
401: `handleUnauthorized()` refreshes once, the stream re-fetches once
with the same method, body, and signal, and values pump as if nothing
happened. Session death surfaces the existing displayable error envelope —
and with slice 03's wiring, the app-level seam has already redirected the
user. An abort while awaiting the refresh propagates as cancellation, not
failure. Demo: a stream whose first response is 401 and whose re-fetch
succeeds pumps its values normally, with exactly one refresh call on the
wire.

## Acceptance criteria

- [ ] `streamNdjson` calls `handleUnauthorized()` on a 401 response: `true` → re-fetches once with the same method, body, and abort signal; `false` → throws the envelope from the existing error mapping
- [ ] A 401 on the re-fetched stream goes straight to session death — no second refresh (the re-fetch is marked)
- [ ] An abort signal firing while awaiting the refresh propagates as an abort error (cancellation, not failure), matching the transport's existing abort contract
- [ ] Transport-level MSW tests: 401 → refresh → re-fetch → values pumped; refresh rejected → seam fired and envelope thrown; abort during refresh propagates abort
- [ ] `useNdjsonOperation` suites are untouched — they surface whatever the transport throws
- [ ] `npx vitest run` is green; touched files pass `npx biome check`

## Blocked by

- `420-401-response-interceptor-02-interceptor-policy`

## Technical notes

- A 401 means the request was rejected before execution, so re-fetching a streamed POST (prune, auto-update) is safe — nothing ran server-side.
- `download.ts` stays outside the policy by decision (parent, decision 1); do not wire it.
- Seam-wiring behavior (logout, redirect) is pinned at App level in slice 03; these transport tests fire a registered test double at most.
