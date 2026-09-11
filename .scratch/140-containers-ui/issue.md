---
title: Containers UI — list, create, lifecycle, streaming logs
parent: none
date: 2026-08-18
status: open
labels:
  - needs-triage
  - frontend
  - containers
  - feature
author: vibe-agent
---

## Problem Statement

As a user, I cannot see or control containers on my machines from the browser. Everyday operations — checking what runs, starting or stopping a container, creating one, reading its logs — all require SSH or curl. Container logs especially need streaming with follow, since a static fetch cannot show a running service's output.

## Solution

Fill the machine detail page's Containers tab: a list of containers with state badges and quick lifecycle actions (start/stop with pending states, delete with confirmation), a create-container form, and a log viewer that streams NDJSON log output with follow and tail controls, reusing the streaming pattern established by the images pull.

## User Stories

1. As a user, I want to see all containers on a machine with name, image, and state, so that I get an overview of what runs there.
2. As a user, I want state communicated with color-coded badges (running green, exited gray, paused yellow, dead red, etc.), so that problems jump out.
3. As a user, I want to start a stopped container, so that I can bring services up.
4. As a user, I want to stop a running container, so that I can take services down.
5. As a user, I want per-container action buttons to show pending state while the operation runs, so that I don't double-click or doubt the click registered.
6. As a user, I want the list to update after a lifecycle action completes, so that the shown state is trustworthy.
7. As a user, I want to delete a container with an explicit confirmation, so that it never happens by accident.
8. As a user, I want to create a container by naming an image and optionally a name and command, so that I can deploy simple services without YAML.
9. As a user, I want validation on the create form (image required), so that obvious mistakes never reach the server.
10. As a user, I want to read a container's logs, so that I can debug and observe it.
11. As a user, I want to follow logs live, so that I see output as it happens.
12. As a user, I want to limit how many past lines I fetch (tail), so that huge logs don't flood the browser.
13. As a user, I want log streaming to stop when I navigate away, so that no background connections leak.
14. As a user, I want a failed lifecycle action to show the backend error code and message, so that I can diagnose (e.g. image missing).
15. As a user, I want loading, empty, and error states on the container list, so that the tab is never ambiguously blank.

## Implementation Decisions

- ContainerList inside the detail page Containers tab, fed by a per-machine `useContainers` query.
- Lifecycle actions (start/stop) as React Query mutations with per-row `isPending`; on success invalidate the containers query.
- Delete behind the shared confirmation modal; also invalidates.
- Create form (react-hook-form): required image, optional name and command; maps to the backend create DTO; success closes the modal and refetches.
- Log viewer: streaming `fetch` NDJSON with `AbortController` reusing the stream helper introduced by the images PRD; follow toggle (default off), tail selector (default 100), auto-scroll while following, cleanup on unmount or when follow is switched off.
- State badge palette defined once in shared formatters/badges.
- No optimistic updates — server state is refetched; React Query cache per machine key hierarchy.

## Testing Decisions

- Same seam: Vitest + RTL + MSW for REST; mocked `ReadableStream` for logs.
- List tests: rendering with each state badge, empty and error states.
- Lifecycle tests (MSW): start/stop posts hit the right URL, pending state on the button, list invalidated on success, error envelope shown on failure.
- Create tests: validation, submit payload shape, modal close + refetch.
- Log tests: initial tail fetch, follow toggling opens/closes the stream, new lines render while following, navigation/unmount aborts the stream, error event surfaces.
- Prior art: skeleton + images MSW suite and stream-mocking helper.

## Out of Scope

- Container stats/inspect views, exec/terminal, pods.
- Renaming or updating containers.
- Live list updates via SSE events (backend stub).
- Log filtering by regex or time range beyond `since` passthrough.

## Further Notes

- Blocked by `130-images-ui` (reuses its streaming helper and progress patterns).
- The shared streaming log viewer introduced here is reused by quadlets logs (PRD 150).
