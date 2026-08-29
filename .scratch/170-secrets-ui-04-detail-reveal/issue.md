---
title: Secrets UI — inspect detail with admin-only value reveal
parent: 170-secrets-ui
date: 2026-08-27
status: closed
labels:
  - ready-for-agent
  - frontend
  - secrets
  - feature
author: vibe-agent
---

## Parent

`.scratch/170-secrets-ui/issue.md`

## What to build

Add a detail view per secret — expandable row or modal (pick whichever matches the tab's density; quadlets used a modal for edit) — seeded from `GET /machines/${machineId}/secrets/${secretId}`, showing ID, name, driver, labels, created/updated timestamps.

The centrepiece is the **reveal** control:

- A "Reveal value" button rendered **only** for `canRevealSecrets` users (admin/owner). Read/write users never see it — the backend 403s the call anyway; the UI simply doesn't offer it (hide-and-hint pattern: a short line explains that revealing requires admin).
- Clicking reveal calls the inspect endpoint with `?reveal=true`; the returned value renders **masked** (`type="password"` styling / masked monospace) with an eye toggle — revealing on screen must be a second deliberate act.
- The value lives only in component state: the detail query must not cache it (use the quadlets `gcTime: 0` detail-query pattern, or fetch the reveal through a mutation so React Query never caches the value), and it is discarded when the view closes.
- Handle 403 gracefully (race: access revoked mid-session) with the shared error display.

## Acceptance criteria

- [ ] Detail view shows ID, name, driver, labels, created/updated from the inspect endpoint
- [ ] Reveal button absent for read/write users; present for admin
- [ ] Revealed value displays masked with an eye toggle; never in plain text by default
- [ ] Value is not retained in any React Query cache after the view closes
- [ ] 404/403/connection errors render via the shared error display
- [ ] MSW tests: detail fields render; reveal request carries `reveal=true` for admin; read user sees hint not button; masked rendering and toggle

## Blocked by

- 170-secrets-ui-01-list
