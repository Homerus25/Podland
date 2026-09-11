---
title: Volumes UI — list, inspect drawer, create, delete, prune, export, populate
parent: none
date: 2026-08-30
status: ready-for-agent
labels:
  - volumes
  - frontend
  - feature
author: vibe-agent
---

## Problem Statement

Podland's backend exposes volume management per machine, but the web UI has no volume surface: users cannot see volumes, inspect their config, create or delete them, back up their contents, or seed them with files without dropping to the podman CLI over SSH. Shared users without SSH access cannot work with volumes at all.

## Solution

Add a **Volumes** tab to the machine detail page (after Containers), backed by the 200-volumes-management endpoints. The tab shows all volumes — named and anonymous — with metadata; clicking a row opens an inspect drawer with the full config; write actions (create modal, delete with force confirmation, prune with report, populate upload modal) follow the existing danger-confirmation patterns from images/containers. Export downloads the volume as a tar via an authenticated fetch. Mutating controls are hidden for read-only users, following the existing permission-hint pattern. Lists refresh via React Query invalidation — no SSE, consistent with containers.

Feature code lives in the standard feature folder with an API client module, React Query hooks, and components; MSW-based tests cover hooks and components including permission-gated states.

## User Stories

1. As a user with read access, I want to see all volumes on a machine with name, type badge (named/anonymous), driver, creation date, and size when reported, so that I can survey persistent data at a glance.
2. As a user with read access, I want distinct loading, empty, and error states on the volumes list, so that the tab is never ambiguously blank.
3. As a user with read access, I want to click a volume to see its full config (labels, options, driver, mountpoint, created), so that I can debug without SSH.
4. As a user with write access, I want to create a volume with name and labels, hiding driver/options behind an advanced section, so that the common case stays simple.
5. As a user with write access, I want to omit the name and still create an anonymous volume, learning its generated name, so that all podman workflows work.
6. As a user with write access, I want to delete a volume behind a danger confirmation with an explicit force checkbox, so that cleanup is deliberate and in-use volumes can still be removed knowingly.
7. As a user with write access, I want prune behind a danger confirmation with a report of what was removed and reclaimed, so that bulk cleanup is transparent.
8. As a user with read access, I want to export a volume as a tar download, so that I can back up data from machines I cannot modify.
9. As a user with write access, I want to populate a volume by picking multiple files or uploading a tar, with a clear notice that contents merge, so that seeding is predictable.
10. As a user with read access, I want mutating controls hidden rather than erroring on click, so that I never start an action I cannot finish.
11. As a user, I want the list to refresh after every mutation, so that the UI reflects the machine's real state.

## Implementation Notes

- Backend contract and rulings: `.scratch/200-volumes-management/200-0-volumes-management-master.md`
- Structure mirrors the secrets/containers UI epics: feature folder with `services/` API client, `hooks/`, and `components/`; one slice per operation, 1:1 with the backend issues.
- Export uses an authenticated fetch → blob → save flow (axios instance token handling); a small shared download utility is acceptable.
- Upload (populate) builds `multipart/form-data` with relative paths from the file picker.

## Out of Scope

- Volume rename, replace-mode populate, file browsing inside volumes, quadlet volume units, SSE-driven auto-refresh (all per parent PRD rulings).

## Slices

1. `210-volumes-ui-01-list` — Volumes tab + list table — blocked by 200-01
2. `210-volumes-ui-02-inspect-drawer` — inspect drawer — blocked by 200-02, 210-01
3. `210-volumes-ui-03-create-modal` — create modal — blocked by 200-03, 210-01
4. `210-volumes-ui-04-delete` — delete + force confirm — blocked by 200-04, 210-01
5. `210-volumes-ui-05-prune` — prune + report — blocked by 200-05, 210-01
6. `210-volumes-ui-06-export` — tar download — blocked by 200-06, 210-01
7. `210-volumes-ui-07-populate` — upload modal — blocked by 200-07, 210-01
