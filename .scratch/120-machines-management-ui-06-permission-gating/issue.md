---
title: "Permission-aware controls across machines list and detail"
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

A consistent, derived-permission gating pass over all machine controls once the mutating features exist. Effective permission treats the owner as admin. Rules: read users get no write actions (edit, delete, share) anywhere, hidden or disabled with a brief explanation; write users can edit metadata but see no sharing or delete controls; owner/admin sees everything. The machines list and detail page agree on these rules.

This gating is a derived-UI concern only; the backend remains the enforcement point. Where earlier slices applied local gating, unify it behind one helper so rules live in one place.

## Acceptance criteria

- [ ] A single helper derives effective permission (owner → admin) from machine + current user
- [ ] Read-permission users see no edit, delete, or share controls on list or detail, with an explanatory hint where controls would be
- [ ] Write-permission users can edit metadata but cannot delete or share (controls hidden/disabled with explanation)
- [ ] Owner/admin users see all controls
- [ ] List and detail gating agree; no baited 403s
- [ ] MSW tests cover read, write, and admin/owner users across list and detail pages

## Blocked by

- `.scratch/120-machines-management-ui-02-detail-page-shell/issue.md` (and, to be meaningful, slices 03–05)
