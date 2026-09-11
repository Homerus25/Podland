---
title: Frontend scaffold — Vite/React/TS + Chakra + MSW test harness
parent: 110-frontend-skeleton
date: 2026-08-19
status: closed
labels:
  - ready-for-agent
  - frontend
  - foundation
  - feature
author: vibe-agent
---

## Parent

110-frontend-skeleton — Frontend walking skeleton — login, app shell, read-only machines list

## What to build

The frontend project skeleton that every later slice builds on: a Vite + React + TypeScript (strict) app in a new `frontend/` directory, following the feature-sliced layout (`features/`, `shared/`, `pages/`) with the `@`/`@features`/`@shared` path aliases.

Chakra UI v2 is set up with the documented brand theme and `ColorModeScript`, defaulting to the system color-mode preference. A React Query v5 client is created with the documented defaults (no refetch on focus, 5 min stale time, mutations fail fast — no retries) and devtools enabled in dev only. React Router v6 is wired with a single placeholder route, and one axios instance exists as the app's API client. The Vite dev server runs on port 3000 and proxies `/api` to the backend on 8080.

The test harness lands in the same slice: Vitest with jsdom, React Testing Library, and MSW intercepting at the network level — the single test seam the whole skeleton standardizes on. The slice proves the seam with one smoke test that sends a request through the real axios client against an MSW handler and asserts the response.

No login, no protected routes, no real pages yet — this slice is demoable as: dev server boots, placeholder route renders themed, smoke test goes green through MSW.

## Acceptance criteria

- [x] Vite + React + TypeScript (strict) app exists under `frontend/`, builds cleanly, and serves on port 3000 with `/api` proxied to the backend on 8080
- [x] Chakra UI v2 configured with the brand theme and `ColorModeScript`; default color mode follows the system preference
- [x] Path aliases `@`, `@features`, `@shared` resolve in dev server, production build, editor, and tests
- [x] React Query v5 client created with the documented defaults; devtools present in dev only
- [x] React Router v6 renders a placeholder route
- [x] A single shared axios instance exists as the API client
- [x] Vitest + jsdom + React Testing Library + MSW harness in place; smoke test exercises the real axios client through an MSW handler and asserts the response
- [x] Typecheck, production build, and test suite all pass via documented npm scripts

## Blocked by

None - can start immediately
