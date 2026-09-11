---
title: Volume create modal
parent: Volumes UI — list, inspect drawer, create, delete, prune, export, populate
date: 2026-08-30
status: ready-for-agent
labels:
  - volumes
  - frontend
  - feature
author: vibe-agent
---

## Parent

Volumes UI epic (`.scratch/210-volumes-ui/issue.md`); backend contract in `.scratch/200-volumes-management/200-0-volumes-management-master.md`.

## What to build

A "Create volume" button in the Volumes tab (visible only to write-level users, following the existing permission-hint pattern) opening a modal: name (optional — placeholder text explains that omitting it creates an anonymous volume with a generated name) and a labels key/value editor are visible; driver and driver options sit behind a collapsed "Advanced" section. On success the modal closes, the volumes query is invalidated, and a toast confirms — including the generated name for anonymous volumes.

Client-side validation matches the backend contract; backend validation errors (e.g. podman-rejected names) surface inline with their message.

## Acceptance criteria

- [ ] Create button is hidden for read-level users
- [ ] Modal submits name (optional), labels, and advanced driver/options to the create endpoint
- [ ] Empty name creates an anonymous volume; success toast shows the generated name
- [ ] Volumes list refreshes after successful creation
- [ ] Backend validation errors render inline with messages
- [ ] MSW tests: submit payload shape, anonymous path toast, invalidation, hidden button for read users

## Blocked by

- 200-volumes-management-03-create (create endpoint)
- 210-volumes-ui-01-list (Volumes tab and list)
