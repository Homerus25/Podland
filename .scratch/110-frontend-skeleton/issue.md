---
title: Frontend walking skeleton — login, app shell, read-only machines list
parent: none
date: 2026-08-18
status: open
labels:
  - needs-triage
  - frontend
  - foundation
  - feature
author: vibe-agent
---

## Problem Statement

As a user of Podland, there is no user interface at all — every action (registering a machine, starting a container) requires curl against the API. I cannot see my machines, and I cannot use the app from a browser. The existing implementation guide (`doku/description_frontend.md`) also assumes a cookie-session auth model and a public register page, neither of which matches the actual backend (JWT Bearer tokens, admin-only user creation), so building straight from the doc would produce a broken login.

## Solution

A walking skeleton that proves the full pipeline end-to-end: a Vite + React + TypeScript app where I can log in with username and password, stay logged in across page refreshes via the refresh-session flow from PRD `100`, land in a shell with sidebar navigation and header, and see a read-only list of my machines as cards. Includes dark mode. Everything risky (auth token handling, API client, error envelope rendering, test harness) gets exercised by this one thin slice; feature PRDs (machines management, images, containers, quadlets) build on it.

## User Stories

1. As a visitor with no session, I want to be redirected to the login page when opening any app URL, so that private data is never flashed unauthenticated.
2. As a visitor, I want to return to the page I originally requested after logging in, so that deep links keep working.
3. As a user, I want to log in with my username and password, so that I can access my machines.
4. As a user, I want a clear error message when my credentials are wrong, so that I know to retry rather than wonder what broke.
5. As a user, I want to stay logged in when I refresh the page or reopen the browser, so that the dashboard feels like a tool, not a form.
6. As a user, I want the silent refresh to happen without any visible flash or spinner-then-content flicker beyond a single initial loading state, so that reloads feel instant.
7. As a user, I want requests that hit an expired access token to retry automatically after a refresh, so that I never see spurious "session expired" errors mid-work.
8. As a user on a session that is truly gone (revoked or expired), I want to be returned to the login page, so that the app doesn't sit in a broken state.
9. As a user with many tabs open, I want concurrent expired requests to trigger exactly one refresh, so that my session doesn't race itself into invalidation.
10. As a user, I want my access token to never touch localStorage, so that XSS cannot exfiltrate a bearer credential.
11. As a user, I want to see my machines as cards with name, description, hostname:port, and my permission badge, so that I can recognize and distinguish them at a glance.
12. As a user with no machines yet, I want an empty state that tells me how to get started, so that a blank screen never confuses me.
13. As a user, I want loading and error states on the machines list, so that I can tell "loading" from "broken" from "empty".
14. As a user, I want backend errors shown with their error code and human message (and generic text for server-internal errors), so that I can report issues usefully.
15. As a user, I want to log out, so that I can end my session on a shared computer.
16. As a user, I want to toggle between light and dark mode (defaulting to my system preference), so that the app is comfortable to look at.
17. As a user, I want sidebar navigation (Dashboard, Machines) and a header with my username, so that I always know where I am and who is logged in.
18. As a developer, I want a single network-level test seam, so that the auth client and error handling are tested without hand-rolled per-service mocks.

## Implementation Decisions

- Stack per the implementation guide: Vite, React 18, TypeScript strict, Chakra UI v2 with the documented brand theme and `ColorModeScript`, React Query v5 with the documented client defaults (no refetch on focus, 5 min stale time, mutations fail fast), React Router v6, axios for REST.
- Folder structure follows the guide's feature-sliced layout (`features/`, `shared/`, `pages/`) with the `@`/`@features`/`@shared` path aliases.
- Auth model corrected to the real backend: access JWT held in memory only (auth context), injected as a Bearer header by an axios request interceptor; never persisted to storage. The refresh token lives exclusively in the httpOnly cookie from PRD 100.
- Silent refresh: on app mount, before first render of protected content, call `/api/auth/refresh` to obtain an access token; treat failure as logged-out.
- 401 handling in a response interceptor: queue concurrent failures behind a single in-flight refresh, retry each failed request once with the new token; if the refresh itself 401s, clear auth state and redirect to login preserving the return location.
- Login form uses react-hook-form with inline validation (settles the guide's open question).
- No register page — backend user creation is admin-only via API; an admin user-management UI may be a future PRD.
- Error handling: a global error context renders the backend error envelope (`requestId`, `error.message`, `error.code`); internal/server error codes get a generic message, others show the backend message.
- Routing: public `/login`; protected shell (Header + Sidebar layout) wrapping Dashboard and Machines pages; `ProtectedRoute` shows a full-page spinner while initial auth state resolves.
- Machines list is read-only: `useMachines` query + card components (name, description, hostname:port, permission badge). Create/edit/delete/share arrive in PRD 120.
- UI primitives are built only as the skeleton actually consumes them (Button, Input, Card, Badge, Alert, Spinner, Modal not needed yet); no upfront component kit, no Storybook, no i18n (settles remaining guide open questions).
- Dark mode: Chakra `useColorMode`, system default, preference in localStorage.
- Dev setup: Vite dev server on port 3000 proxying `/api` to the backend on 8080.

## Testing Decisions

- Single seam: MSW intercepts at the network level, so the real axios client, interceptors, token injection, and refresh logic are all under test; component tests use React Testing Library and assert rendered behavior (text, roles, navigation), never implementation details.
- Test runner: Vitest, jsdom.
- Auth flow tests (MSW): successful login stores user state; failed login shows the envelope error; silent refresh on mount succeeds/fails; a 401 on a data request triggers refresh then retry; concurrent 401s produce exactly one refresh call; refresh failure redirects to login and preserves the return location.
- Machines list tests (MSW): loading state, rendered cards, empty state, error state with code/message.
- Routing tests: unauthenticated access to a protected deep link redirects to login and returns after login.
- MSW handlers mirror the real API shapes from the backend integration tests, including the error envelope.

## Out of Scope

- Machine create/edit/delete and sharing UI (PRD `120-machines-management-ui`).
- Machine detail page and all domain tabs (containers, images, quadlets).
- SSE events (backend endpoint is a 501 stub).
- Offline detection and the offline banner.
- Register page and admin user management.
- Storybook, i18n, Playwright E2E, mobile-specific layouts beyond Chakra defaults.

## Further Notes

- Blocked by `100-auth-refresh-sessions` (the skeleton consumes the refresh endpoint; with only today's backend, silent refresh cannot work).
- This PRD deliberately diverges from `doku/description_frontend.md` on auth (Bearer + refresh cookie instead of `withCredentials` sessions) and drops the RegisterPage; the doc remains the reference for everything else.
- React Query devtools enabled in dev only.
