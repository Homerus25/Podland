---
title: Persistent session — silent refresh, single-flight 401 retry, dead-session redirect
parent: 110-frontend-skeleton
date: 2026-08-19
status: open
labels:
  - ready-for-agent
  - frontend
  - auth
  - feature
author: vibe-agent
---

## Parent

110-frontend-skeleton — Frontend walking skeleton — login, app shell, read-only machines list

## What to build

The slice that makes the dashboard feel like a tool instead of a form: staying logged in.

On app mount, before the first render of any protected content, the app calls `POST /api/auth/refresh`; the browser attaches the httpOnly `refresh_token` cookie automatically (it is scoped to `/api/auth` and never visible to JavaScript). Success returns a fresh access JWT (`{ "token": ... } }`) and the user is logged in without seeing a form; failure means logged out and the existing login redirect applies. The only permitted loading UI is the single full-page spinner from the shell — no flash of login form, no spinner-then-content flicker.

An axios response interceptor handles expired access tokens: when a data request gets a 401, it triggers one refresh; concurrent 401s queue behind that single in-flight refresh (no races that invalidate the session by refreshing twice); each failed request is retried exactly once with the new token. If the refresh itself fails (session revoked or expired), the auth state is cleared and the user is redirected to `/login` preserving the return location. The access token remains memory-only throughout.

## Acceptance criteria

- [ ] On mount with a valid refresh cookie, the silent refresh completes before protected content renders; the user is logged in without seeing the login form
- [ ] On mount with no/gone session, the user lands on `/login`; deep-link return location still works
- [ ] The only loading UI during initial auth resolution is the single full-page spinner — no login-form flash
- [ ] A 401 on a data request triggers a refresh and the original request is retried once with the new token
- [ ] Concurrent 401s produce exactly one refresh call; every queued request is retried with the new token
- [ ] Refresh failure (revoked/expired session) clears auth state and redirects to `/login` preserving the return location
- [ ] The access token is never written to localStorage or sessionStorage (memory only)
- [ ] MSW tests: silent refresh success/failure on mount; 401-then-retry; single-flight under concurrent 401s; refresh-failure redirect with preserved location
- [ ] Typecheck, build, and tests pass

## Blocked by

- 110-frontend-skeleton-02-app-shell-logout
