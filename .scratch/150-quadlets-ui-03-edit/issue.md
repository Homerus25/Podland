---
title: Quadlets UI — view and edit quadlet content
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

Add an edit flow to the Quadlets tab: an action per list row opens the quadlet in the shared create-form component, seeded from the quadlet get endpoint. Because the backend derives both the name (from the URL) and the resource type (from the existing file path) on update, the seeded name and resource type are shown read-only and only the content textarea is editable.

Opening the editor shows a loading state while the content is fetched and an error state (envelope code + message) if the quadlet disappears or the ssh connection fails. Saving sends the content-only update payload to the quadlet's update endpoint; on success the editor closes and the quadlet list refetches (state may change after a content change and systemd reload).

## Acceptance criteria

- [ ] An edit action on each quadlet row opens the editor seeded with that quadlet's current content
- [ ] Seeded name and resource type render read-only; only content is editable
- [ ] Content loading and error states are distinct; errors show the envelope code and message
- [ ] Saving sends the content-only payload and closes the editor on success
- [ ] Quadlet list refetches after a successful update
- [ ] Vitest + RTL + MSW tests cover: seeded content renders, content-only request payload, list refetch on success, and error envelope display

## Blocked by

- `.scratch/150-quadlets-ui-02-create/issue.md`
