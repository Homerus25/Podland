---
title: "Frontend: quadlet auto-update actions with live progress"
parent: 190-quadlets-auto-update
date: 2026-08-30
status: ready-for-agent
labels:
  - ready-for-agent
  - quadlets
  - frontend
  - feature
author: vibe-agent
---

## Parent

`.scratch/190-quadlets-auto-update/issue.md` — Quadlet auto-update with live streaming output

## What to build

The quadlets panel on the machine detail page gains two toolbar actions:

- **"Check for updates"** — runs the auto-update endpoint in dry-run mode immediately, no confirmation (it mutates nothing).
- **"Update all"** — opens a confirmation dialog explaining that containers with an auto-update policy will be updated and restarted; on confirm, runs the real update.

Both consume the streaming endpoint through a mutation hook built on the existing shared NDJSON stream reader (same consumption pattern as the image pull). While running, a progress modal shows the live output lines as they arrive and an in-progress state so the user knows the run hasn't stalled. On the terminal complete event, the modal renders the parsed per-container report: which containers updated, which failed with their errors, and — for dry runs — which have pending updates with their images. The modal clearly distinguishes dry-run from real run, and a terminal error event (e.g. machine unreachable) is surfaced as a distinct connection error, not as "update failed".

On completion of a real run, the quadlet, container, and image queries are invalidated so the UI reflects restarted containers and new images without a manual reload.

Both buttons render only for users with write access to the machine (existing permission gating helpers); read-only users see neither. The machines overview cards stay untouched — no auto-update action there.

## Acceptance criteria

- [ ] Toolbar buttons visible only with write access to the machine; read-only users see neither
- [ ] "Update all" requires confirmation before the API call fires; "Check for updates" fires directly
- [ ] Live output lines render as they arrive from the stream, with an in-progress state while the stream is open
- [ ] Terminal complete event renders the per-container report: updated / failed with errors / pending (dry-run), including images
- [ ] Dry-run and real run are visually distinguished; a terminal error event renders as a connection error distinct from per-container failures
- [ ] Completion of a real run invalidates the quadlet, container, and image queries
- [ ] Hook and component tests follow the existing feature test patterns (prior art: the image pull streaming hook and the quadlet panel tests): stream consumption, confirmation gating, report rendering, invalidation, permission gating
- [ ] Lint, typecheck, and existing frontend tests pass

## Blocked by

- 190-quadlets-auto-update-02-backend-auto-update-endpoint (the streaming endpoint this UI consumes)
