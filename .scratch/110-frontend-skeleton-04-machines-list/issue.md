---
title: Read-only machines list — cards with permission badge, loading/empty/error states
parent: 110-frontend-skeleton
date: 2026-08-19
status: open
labels:
  - ready-for-agent
  - frontend
  - feature
author: vibe-agent
---

## Parent

110-frontend-skeleton — Frontend walking skeleton — login, app shell, read-only machines list

## What to build

The first real data page, and the slice that completes the walking skeleton: seeing your machines from a browser.

The Machines page (already in the sidebar from the shell slice) fetches `GET /api/machines` via a `useMachines` React Query hook and renders each machine as a card showing its name, description, hostname:port, and the current user's permission badge. The page distinguishes its three states clearly: a loading state while fetching, an empty state that tells a user with no machines how to get started, and an error state that renders the backend error envelope (code + human message, generic text for internal/server codes) using the shared error handling established in the login slice.

Strictly read-only — create/edit/delete/share UI arrives in PRD `120-machines-management-ui`; no machine detail page.

## Acceptance criteria

- [ ] Machines page lists one card per machine with name, description, hostname:port, and the user's permission badge
- [ ] Loading state is visually distinct from empty and error states
- [ ] Empty state (no machines) explains how to get started rather than showing a blank screen
- [ ] Error state renders the backend error envelope with code and message; internal/server codes show generic text
- [ ] Cards are recognizable and distinguishable at a glance across multiple machines
- [ ] MSW tests: loading state, rendered cards from a machine list fixture, empty state, error state with code/message
- [ ] Typecheck, build, and tests pass

## Blocked by

- 110-frontend-skeleton-02-app-shell-logout
