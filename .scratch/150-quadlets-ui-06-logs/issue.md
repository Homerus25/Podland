---
title: Quadlets UI — journalctl log viewer wiring
parent: 150-quadlets-ui
date: 2026-08-26
status: open
labels:
  - ready-for-agent
  - frontend
  - quadlets
  - feature
author: vibe-agent
---

## Parent

`.scratch/150-quadlets-ui/issue.md`

## What to build

Add a logs action per quadlet row that opens the shared, generic log viewer (built for containers in `140-containers-ui-05`) wired to the quadlet's journalctl endpoint. The wiring is a quadlet `LogSource`: the tail fetch calls the non-following logs endpoint with the `tail` parameter and maps its response into lines, and the follow function opens the NDJSON stream with `tail` and `follow=true` via the shared stream helper (plain fetch with `AbortController`, no axios).

Closing the viewer or navigating away (component unmount) must abort the stream so no background connections leak — that guarantee is owned by the shared viewer, so this slice's job is correct wiring, not re-implementation. Stream and fetch errors surface with the envelope code + message via the viewer's existing error path.

## Acceptance criteria

- [ ] A logs action on each quadlet row opens the shared log viewer for that quadlet
- [ ] Tail selector maps onto the logs endpoint's `tail` parameter (default 100)
- [ ] Follow toggle opens the NDJSON stream with `tail` and `follow=true` and renders new lines live
- [ ] Closing the viewer or navigating away aborts the stream (no leaked connections)
- [ ] Errors surface the envelope code and message
- [ ] Vitest + RTL + MSW tests cover: the wiring hits the right endpoint with the right params for both tail fetch and follow stream (follow tested via the mocked `ReadableStream` helper)

## Blocked by

- `.scratch/150-quadlets-ui-01-list/issue.md`
