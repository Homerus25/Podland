---
title: Login page with in-memory Bearer auth + error envelope rendering
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

The first user-visible slice: logging in from a browser.

A public `/login` page with a username/password form built on react-hook-form with inline validation (required fields, non-empty credentials). Submitting calls `POST /api/auth/login`, whose response body is `{ "token": "<access JWT>" }`. On success, the access token and username are stored in an auth context held in memory only — never written to localStorage or sessionStorage — and an axios request interceptor starts injecting the token as a `Authorization: Bearer` header on API requests. The user lands on a placeholder dashboard page.

On failure, the backend error envelope is rendered inline on the form: the error `code` and the human-readable `error.message`, with generic text substituted for internal/server error codes. This establishes the error-envelope handling every later feature reuses.

No refresh, no persistence across reloads yet — refreshing the page still lands you back at login (next slices fix that).

## Acceptance criteria

- [x] `/login` is publicly reachable and renders a username/password form with inline validation via react-hook-form
- [x] Successful login stores the access token and username in a memory-only auth context; no auth material appears in localStorage or sessionStorage
- [x] Authenticated API requests carry `Authorization: Bearer <token>` via the axios request interceptor
- [x] Failed login renders the error envelope inline: code + human message; internal/server error codes show generic text
- [x] Successful login navigates to the placeholder dashboard
- [x] MSW tests: successful login stores user state and injects the Bearer header; failed login shows the envelope error (code and message); internal errors show generic text
- [x] Typecheck, build, and tests pass

## Blocked by

- 110-frontend-skeleton-00-scaffold
