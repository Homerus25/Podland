---
title: "Share machine modal: list, grant, and revoke access"
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

A ShareMachineModal opened from the machine detail page Settings tab. It lists the current access grants (username, permission, granted time), lets the owner grant a user access with a permission selector (read / write / admin), and revoke a specific user's access per row.

All actions invalidate the grants (and machines) queries and show pending states per action. Failures render the envelope (e.g. USER_NOT_FOUND, ACCESS_DENIED). The share controls are available only to owner/admin users; write/read users do not see them.

## Acceptance criteria

- [ ] Settings tab exposes a sharing section/modal listing current grants with username, permission, and granted timestamp
- [ ] Granting posts the selected permission (read/write/admin) for the chosen user
- [ ] Revoking removes that user's row and refetches grants
- [ ] Grant list refetches after grant/revoke mutations
- [ ] Failed grant/revoke renders the error envelope with a pending state on the triggering action
- [ ] Only owner/admin users see share controls; write and read users do not
- [ ] MSW tests cover: grants rendered, grant posts selected permission, revoke removes row, hidden for non-admin users, failure envelope

## Blocked by

- `.scratch/120-machines-management-ui-02-detail-page-shell/issue.md`
