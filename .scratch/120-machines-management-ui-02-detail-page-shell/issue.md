---
title: "Machine detail page shell with tabs"
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

A machine detail page at the per-machine route. The header shows the machine identity (name, hostname, port, SSH username, permission badge). The body hosts four tabs: Containers, Images, Quadlets, and Settings. The first three render empty placeholders in this slice; the Settings tab is an empty container that later slices (edit, delete, sharing) populate. Machines list cards gain a "View" action navigating into the detail page.

This slice introduces the single-machine query (fetch by id, with loading and error states matching the list page patterns).

## Acceptance criteria

- [ ] Machines list cards have a "View" action that navigates to the machine detail page
- [ ] Detail page fetches the machine by id with loading and error (envelope) states
- [ ] Header shows machine identity (name, hostname, port, SSH username) and permission badge
- [ ] Tabs Containers / Images / Quadlets / Settings are present and switchable
- [ ] Containers, Images, and Quadlets tabs render empty placeholders
- [ ] Settings tab exists (empty placeholder is fine in this slice)
- [ ] Unknown machine id renders the error envelope (e.g. MACHINE_NOT_FOUND)
- [ ] MSW tests cover: navigation from list, loading, rendered identity, tab switching, not-found error
