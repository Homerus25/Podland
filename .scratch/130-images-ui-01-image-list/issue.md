---
title: Images UI — image list in machine detail Images tab
parent: 130-images-ui
date: 2026-08-21
status: open
labels:
  - ready-for-agent
  - frontend
  - images
  - feature
author: vibe-agent
---

## Parent

`.scratch/130-images-ui/issue.md`

## What to build

Fill the machine detail page's Images tab with a list of all images on the machine. Each row shows the repo tags (dangling/untagged images render as `<none>` plus a short id), a human-readable size (MB/GB), and a human-readable creation date (relative). The list has distinct loading, empty, and error states rendered through the shared error display so "empty" is never confused with "broken".

Data comes from the backend image list endpoint via a `useImages` query keyed per machine, following the existing `useMachines` hook patterns (axios instance, react-query, `getApiError` envelope handling).

## Acceptance criteria

- [ ] Images tab on the machine detail page renders the image list (no more placeholder)
- [ ] Each image row shows repo tags, human-formatted size, and relative creation date
- [ ] Untagged images display as `<none>` with a short id so dangling images stay identifiable
- [ ] Loading, empty, and error states are distinct; errors flow through the shared error envelope display
- [ ] `useImages` hook follows existing feature hook conventions and is keyed per machine
- [ ] Vitest + RTL tests with MSW: rendered fields, human formatting, empty state, error state

## Blocked by

None - can start immediately (detail page shell from 120 is merged).
