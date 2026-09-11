---
title: "Edit machine metadata on the Settings tab"
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

The Settings tab of the machine detail page hosts the MachineForm in edit mode, prefilled with the machine's current metadata. Editing covers name, description, hostname, port, and SSH username. The private key file input is optional in edit mode: leaving it empty sends a partial update without touching the key; uploading a new file re-sends it (connection re-test happens server-side).

Same validation rules as create (required fields, port range). Successful save shows feedback and refetches the machine and the machines list; failures render the envelope. Save button has a pending state.

Controls are gated by permission: visible for owner/admin/write users, hidden for read users (full gating polish lands in the permission-aware slice, but this slice must at least hide the form for read users so story 12 holds once combined with backend enforcement).

## Acceptance criteria

- [ ] Settings tab shows the edit form prefilled with current machine metadata
- [ ] Name, description, hostname, port, and SSH username are editable
- [ ] SSH private key input is optional in edit mode; omitting it does not clear or require the key
- [ ] Uploading a new key file sends it in the update request body
- [ ] Successful save refetches the machine detail and the machines list
- [ ] Failed save renders the error envelope with pending state on the button
- [ ] Read-permission users do not see the edit form
- [ ] MSW tests cover: prefill, edit persists (request payload asserted), key not required, failure envelope, hidden for read user

## Blocked by

- `.scratch/120-machines-management-ui-02-detail-page-shell/issue.md`
