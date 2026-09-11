---
title: Images UI — list, delete, pull with streaming progress
parent: none
date: 2026-08-18
status: open
labels:
  - needs-triage
  - frontend
  - images
  - feature
author: vibe-agent
---

## Problem Statement

As a user managing images on a machine, I cannot see which images exist, pull new ones, or remove unused ones without curl. Image pulls over SSH to remote machines can take minutes, so a plain request/response UI would either block silently or time out — I need live progress and the ability to cancel.

## Solution

Fill the machine detail page's Images tab: a list of images with recognizable names, sizes, and ages; delete with confirmation (surfacing the in-use case); and a pull flow that consumes the backend's NDJSON progress stream to show live status and a progress bar, with cancel via abort. This is the first streaming consumer in the frontend and establishes the streaming-fetch pattern.

## User Stories

1. As a user, I want to see all images on a machine with their repo tags, size, and creation date, so that I can identify what is present and what is stale.
2. As a user, I want images with no tags shown sensibly (e.g. `<none>` or short id), so that dangling images are still identifiable.
3. As a user, I want sizes and dates formatted human-readably (MB/GB, relative dates), so that I don't parse raw bytes or timestamps.
4. As a user, I want to delete an image with a confirmation dialog, so that accidents are impossible.
5. As a user, I want to be told when an image cannot be deleted because containers use it, so that I understand the failure and can choose force explicitly.
6. As a user, I want to force-delete an in-use image after the error explains the risk, so that cleanup is still possible when I really mean it.
7. As a user, I want to pull an image by reference (e.g. `nginx:latest`), so that I can deploy new software to the machine.
8. As a user, I want live progress while pulling (status text and current/total bytes), so that I know it is working and roughly how far along it is.
9. As a user, I want to cancel a running pull, so that I can abort a wrong reference instead of waiting it out.
10. As a user, I want pull failures surfaced with their error code and message, so that auth failures vs network failures are distinguishable.
11. As a user, I want the image list to refresh when a pull completes, so that the new image appears without manual reload.
12. As a user, I want loading and error states on the image list, so that "empty" and "broken" are never confused.

## Implementation Decisions

- ImageList renders inside the machine detail Images tab; `useImages` query keyed per machine.
- Delete via confirmation modal; on `IMAGE_USED_BY_CONTAINERS` the modal offers force; both invalidating the images query.
- Pull uses streaming `fetch` (not axios) with `AbortController`, reading NDJSON lines and parsing progress/complete/error events per the backend contract (`operationId`, `state.current/total/status/message`); axios remains the REST client everywhere else.
- Progress UI: status line + progress bar + cancel button; on `complete` invalidate the images query; on `error` route the envelope error into the global error display.
- Malformed NDJSON lines are logged and skipped, never fatal.
- One pull at a time per form instance; starting a new pull aborts the previous one.
- `usePullImage` owns stream state (progress event, isPulling, abort) and is reusable by future streaming flows.
- Client disconnect (abort) must actually close the stream so the backend cancels the operation (`OPERATION_CANCELLED` path).

## Testing Decisions

- Same seam: Vitest + RTL; REST interactions via MSW.
- Streaming is tested with a mocked `fetch` returning a `ReadableStream` that emits NDJSON chunks: progress events update the UI; `complete` triggers list invalidation; `error` shows the envelope error; abort closes the reader and cancels the stream; malformed lines are tolerated; a chunk split across reads is reassembled correctly.
- List tests: rendered fields, human formatting, empty state, error state, delete confirmation, force path after in-use error.
- Prior art: skeleton MSW handlers; this PRD adds the first stream-mocking helper, which later log viewers reuse.

## Out of Scope

- Registry credentials in the pull form (backend DTO supports them; UI can come later).
- Image build, push, prune, history, tagging (no backend endpoints).
- Live list updates via SSE events (backend stub).
- Image inspection details page.

## Further Notes

- Blocked by `120-machines-management-ui` (detail page shell).
- Sequenced before containers (user decision): the pull stream is the frontend's riskiest new pattern and lands here first; the container log viewer (PRD 140) reuses the stream helper.
