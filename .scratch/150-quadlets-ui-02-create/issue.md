---
title: Quadlets UI — create form with content editor
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

Add a create flow to the Quadlets tab: a button on the list opens a modal form with the quadlet name (without extension), a resource type selector limited to the backend's allowed values (container, pod, image, volume, network), and a plain monospace textarea for the INI content — no syntax highlighting or content validation beyond non-emptiness, per the PRD's implementation decision.

Client-side validation blocks submission until name, resource type, and non-empty content are present, so malformed input never reaches the server. A valid submit posts `{name, resource, content}` to the machine's quadlet create endpoint; on success the modal closes and the quadlet list refetches so the new unit appears. Backend failures (invalid name, quadlet already exists, ssh/systemd errors) surface with the envelope code + message inside the form.

The form component should be written for reuse: the follow-up edit slice seeds it with an existing quadlet's content rather than duplicating the layout.

## Acceptance criteria

- [ ] Create button on the quadlet list opens a modal with name, resource type selector, and monospace content textarea
- [ ] Resource type options are exactly container, pod, image, volume, network
- [ ] Submit is blocked with visible messages until name, resource type, and non-empty content are all provided
- [ ] Valid submit posts the `{name, resource, content}` payload and closes the modal on success
- [ ] Quadlet list refetches after a successful create
- [ ] Backend error responses surface the envelope code and message in the form
- [ ] Vitest + RTL + MSW tests cover: validation messages per required field, request payload shape, list refetch on success, and error envelope display

## Blocked by

- `.scratch/150-quadlets-ui-01-list/issue.md`
