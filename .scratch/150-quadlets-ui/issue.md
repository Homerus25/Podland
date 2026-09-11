---
title: Quadlets UI — core CRUD, lifecycle, logs
parent: none
date: 2026-08-18
status: open
labels:
  - needs-triage
  - frontend
  - quadlets
  - feature
author: vibe-agent
---

## Problem Statement

As a user managing services via quadlets, I cannot see which quadlet units exist on a machine, edit their content, manage their systemd lifecycle (start/stop/restart), or read their journalctl logs from the browser. Quadlet files are the declarative heart of the deployment model, so managing them currently means SSH.

## Solution

Fill the machine detail page's Quadlets tab with the core quadlet surface: list with resource type and state, create and edit of quadlet content in a monospace editor, delete with stop/force options, start/stop/restart lifecycle actions, and a journalctl log viewer (follow/tail) reusing the shared streaming log viewer.

## User Stories

1. As a user, I want to see all quadlets on a machine with name, resource type (container/pod/volume/network/image), and state, so that I can survey my declarative services.
2. As a user, I want quadlet states color-coded (running green, stopped gray, error red, unknown neutral), so that failures stand out.
3. As a user, I want to create a quadlet by giving name, resource type, and INI content, so that I can add services declaratively.
4. As a user, I want required-field validation (name, resource type, non-empty content) on the create form, so that malformed input never reaches the server.
5. As a user, I want to view and edit an existing quadlet's content, so that I can adjust configuration without SSH.
6. As a user, I want to delete a quadlet, with the option to stop it first, so that cleanup handles both running and stopped units.
7. As a user, I want to start, stop, and restart a quadlet, so that I control its lifecycle like a service.
8. As a user, I want pending states on lifecycle buttons, so that slow systemd operations don't invite double clicks.
9. As a user, I want lifecycle results reflected in the list state, so that what I see matches the machine.
10. As a user, I want to read a quadlet's journalctl logs with follow and tail, so that I can debug services exactly like on the console.
11. As a user, I want log streaming to stop when I navigate away, so that connections don't leak.
12. As a user, I want backend errors (not found, ssh failure, systemd errors) surfaced with code and message, so that failures are diagnosable.
13. As a user, I want loading, empty, and error states on the quadlet list, so that the tab is never ambiguously blank.

## Implementation Decisions

- QuadletList inside the detail page Quadlets tab, per-machine query.
- Content editing is a plain monospace textarea — no syntax highlighting or YAML/INI validation library in this PRD.
- Create form: name, resource type selector, content textarea; edit reuses the same form seeded with current content.
- Delete offers the stop-first option; force handling mirrors the backend query parameters.
- Lifecycle start/stop/restart as mutations with pending state and list invalidation; stop/restart may pass timeout/force options with sensible defaults.
- Logs reuse the shared streaming log viewer from the containers PRD (journalctl endpoint with follow/tail parameters).
- State badge palette shared with containers.

## Testing Decisions

- Same seam: Vitest + RTL + MSW; stream via the mocked `ReadableStream` helper.
- List tests: rendering per resource type and state badge, empty/error states.
- Create/edit tests: validation messages, payload shape, refetch on success.
- Delete test: confirmation, stop-first option reflected in the request.
- Lifecycle tests (MSW): correct URLs and query params, pending states, invalidation, error envelope display.
- Log tests: covered by the shared viewer's own suite plus one wiring test per quadlet (right endpoint, params).
- Prior art: images and containers MSW suites.

## Out of Scope

- Generating systemd units from quadlets (backend exists; follow-up PRD).
- Importing existing quadlet files from the machine (backend exists; follow-up PRD).
- Live state updates via SSE events (backend stub).
- Content validation/highlighting, side-by-side diff, generation history.

## Further Notes

- Blocked by `140-containers-ui` (shared log viewer).
- Generate and import were consciously deferred (user decision): they are power-user flows and can land later without rework. They are the natural content of a follow-up PRD (e.g. `160-quadlets-generate-import`).
