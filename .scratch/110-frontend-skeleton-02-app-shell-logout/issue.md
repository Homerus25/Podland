---
title: Protected app shell — routing guard, header/sidebar, logout, dark-mode toggle
parent: 110-frontend-skeleton
date: 2026-08-19
status: closed
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

The app shell and its security perimeter.

A protected layout wraps the app pages: a header showing the logged-in username with a light/dark mode toggle (Chakra `useColorMode`, system default, preference persisted in localStorage), and a sidebar with Dashboard and Machines navigation entries (placeholder pages are fine here).

`ProtectedRoute` guards every non-login URL: while initial auth state resolves it shows a full-page spinner; once resolved, an unauthenticated visitor is redirected to `/login` with the originally requested location preserved, so deep links keep working and private data is never flashed. After a successful login, the user is returned to the page they originally requested.

The header's logout button calls `POST /api/auth/logout` (revoking the server-side refresh session), clears the in-memory auth state, and returns the user to `/login`.

Still no persistence across reloads in this slice — the refresh flow arrives in the next slice; here, a reload of a protected page while "logged in" simply resolves to logged-out and redirects to login.

## Acceptance criteria

- [x] Protected shell layout renders header (username, color-mode toggle) + sidebar (Dashboard, Machines) around page content
- [x] Unauthenticated access to any protected deep link redirects to `/login` preserving the return location; after login the user lands back on the originally requested page
- [x] `ProtectedRoute` shows a full-page spinner while initial auth state resolves
- [x] Color-mode toggle flips light/dark, defaults to system preference, and persists the choice in localStorage
- [x] Logout calls `POST /api/auth/logout`, clears the in-memory auth state, and lands on `/login`
- [x] MSW/routing tests: deep-link redirect with preserved location, return-after-login, logout flow calls the endpoint and clears state
- [x] Typecheck, build, and tests pass

## Blocked by

- 110-frontend-skeleton-01-login-bearer
