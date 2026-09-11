---
title: "Delete machine with confirmation dialog"
parent: 120-machines-management-ui
date: 2026-08-21
status: open
labels:
  - ready-for-agent
  - frontend
  - machines
  - feature
author: vibe-agent
---

## Parent

`.scratch/120-machines-management-ui/issue.md`

## What to build

A delete action on the machine detail page (Settings tab) and/or the machines list, guarded by an explicit confirmation dialog using the shared confirmation modal (the Modal component from the create slice). Confirming issues the delete request; on success the machines queries are invalidated and, when triggered from the detail page, the user is navigated back to the machines list. Failures render the envelope.

The delete control is hidden or disabled with an explanation for users without sufficient permission (owner/admin); full gating consistency lands in the permission-aware slice.

## Acceptance criteria

- [ ] Delete control on the machine detail page requires an explicit confirmation dialog before the request fires
- [ ] Confirming sends the delete request; cancelling dismisses without a request
- [ ] On success the machines list is refetched and the user is navigated back from the detail page
- [ ] Failure renders the error envelope
- [ ] Delete button shows a pending state during the request
- [ ] Users without owner/admin permission do not get a usable delete action
- [ ] MSW tests cover: confirmation required (no request on cancel), success (refetch + navigation), failure envelope

## Blocked by

- `.scratch/120-machines-management-ui-02-detail-page-shell/issue.md`
