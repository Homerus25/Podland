---
title: Containers UI — log viewer with tail and follow streaming
parent: 140-containers-ui
date: 2026-08-26
status: open
labels:
  - ready-for-agent
  - frontend
  - containers
  - feature
author: vibe-agent
---

## Parent

`.scratch/140-containers-ui/issue.md`

## What to build

Add a per-container log viewer opened from the container list. On open it performs an initial fetch of the last N lines, controlled by a tail selector (default 100) that maps onto the logs endpoint's tail parameter. A follow toggle (default off) switches the viewer to live mode: a streaming `fetch` with `AbortController` consumes the backend's NDJSON log stream using the shared stream helper established by the images pull — no axios — appending new lines as they arrive and auto-scrolling while following.

Toggling follow off, closing the viewer, or navigating away (component unmount) aborts the stream so no background connections leak. Stream error events surface the envelope code + message. The viewer should be written as a generic log-streaming component (props for source and controls) rather than container-specific, because quadlets logs (PRD 150) will reuse it.

## Acceptance criteria

- [ ] Opening the viewer fetches and renders the last N lines, with the tail selector respected (default 100)
- [ ] Follow toggle defaults to off; enabling it opens the NDJSON stream and new lines render live
- [ ] The view auto-scrolls to the newest line while following
- [ ] Disabling follow, closing the viewer, or unmounting aborts the stream (no leaked connections)
- [ ] Error events surface the envelope code and message
- [ ] The viewer is a generic, reusable component not coupled to container specifics
- [ ] Vitest + RTL tests with a mocked `ReadableStream` cover: initial tail fetch, follow toggle opening/closing the stream, new lines rendering while following, abort on unmount/navigation, and error event surfacing

## Blocked by

- `.scratch/140-containers-ui-01-list/issue.md`
