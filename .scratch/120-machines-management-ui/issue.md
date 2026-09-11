---
title: Machines management UI — create, edit, delete, share, detail shell
parent: none
date: 2026-08-18
status: open
labels:
  - needs-triage
  - frontend
  - machines
  - feature
author: vibe-agent
---

## Problem Statement

As a machine owner, I can see my machines in the skeleton but I cannot do anything with them from the browser: registering a new machine (including uploading its SSH private key), fixing a typo in a hostname, sharing a machine with another user, or removing a machine I no longer use all require curl. Users with whom a machine has been shared also see edit/delete controls they cannot actually use, with no indication of their permission level beyond a badge.

## Solution

Complete the machines arc in the UI: an "Add Machine" flow with a validated form including SSH private key file upload, edit and delete with confirmation on the machines pages, a share modal managing access grants (list, grant with read/write/admin permission, revoke), permission-aware controls (read users get no write actions), and a machine detail page shell with tabs (Containers, Images, Quadlets, Settings) that later PRDs fill in.

## User Stories

1. As a machine owner, I want to register a machine by filling in name, hostname/IP, port, SSH username, and uploading the SSH private key file, so that I can manage its Podman from the app.
2. As a machine owner, I want optional fields (description, custom podman socket path) to stay optional, so that registration is quick for standard setups.
3. As a machine owner, I want inline validation (required fields, port numeric range) before submit, so that I don't hunt failures in server errors.
4. As a machine owner, I want the SSH key file read client-side and sent in the request body over our HTTPS same-origin connection, so that it is never e-mailed or stored anywhere silly.
5. As a machine owner, I want a clear error (with code) when registration fails (e.g. unreachable host, bad key), so that I can fix the cause.
6. As a machine owner, I want to edit a machine's name, description, hostname, port, and SSH username without re-uploading the private key, so that routine fixes are painless.
7. As a machine owner, I want to delete a machine with an explicit confirmation dialog, so that a stray click cannot destroy access.
8. As a machine owner, I want to see who has access to my machine and with which permission, so that I keep control over sharing.
9. As a machine owner, I want to grant a user access with read, write, or admin permission, so that collaborators get exactly the powers they need.
10. As a machine owner, I want to revoke a user's access, so that former collaborators lose it immediately.
11. As a shared user with read permission, I want write actions (edit, delete, share) hidden or disabled with an explanation, so that I am not baited into 403s.
12. As a shared user with write permission, I want to edit machine metadata but not sharing, so that permissions behave as documented.
13. As a user, I want the machine list to refresh after create/edit/delete/share actions, so that it always reflects reality.
14. As a user, I want to open a machine into a detail page, so that I can work on one machine's containers, images, and quadlets (tabs arrive filled in later PRDs).
15. As a user, I want the detail page's Settings tab to host sharing management, so that machine-level administration lives in one place.
16. As a user, I want pending states on all mutating buttons, so that double submits are prevented.

## Implementation Decisions

- MachineForm (create and edit modes) with react-hook-form; SSH key as a file input read via the File API into the request body; edit mode sends partial updates and does not require the key.
- Create flow opens in a modal from the machines page; edit lives on the detail page Settings tab.
- Delete uses the shared confirmation modal; success invalidates the machines queries and navigates back on the detail page.
- Sharing: ShareMachineModal on the Settings tab — lists grants, grants with a permission selector (read/write/admin), revokes per user; all permission-aware actions gated by the current user's permission on that machine (owner/admin can share; write cannot).
- Permission gating is a derived-UI concern only; the backend remains the enforcement point (machine access middleware).
- MachineDetailPage shell: header with machine identity, tabs Containers / Images / Quadlets / Settings; the first three render empty placeholders in this PRD and are filled by PRDs 130/140/150.
- All mutations via React Query mutation hooks invalidating the machines query keys; per-button `isPending` states.
- Machines list cards gain "View" navigation into the detail page.

## Testing Decisions

- Same seam as the skeleton: Vitest + React Testing Library + MSW at the network level; behavior-only assertions.
- Form tests: required-field and port validation messages; file input presence and required-ness in create mode; not required in edit mode.
- Mutation tests (MSW): create success closes the modal and refetches the list; create failure shows the envelope error; edit persists; delete requires confirmation and refetches.
- Sharing tests (MSW): grant posts the selected permission; revoke removes the row; a read-permission user sees no share/edit/delete controls.
- Prior art: the skeleton's MSW handler set and auth/list tests.

## Out of Scope

- A "test connection" button (no backend endpoint exists for it).
- Content for the Containers, Images, and Quadlets tabs (PRDs 130, 140, 150).
- SSH key rotation UI beyond re-upload during edit (possible via update but not surfaced as its own flow).
- Machine health/status indicators (needs the events backend).
- Admin-level user management.

## Further Notes

- Blocked by `110-frontend-skeleton`.
- Order in the frontend arc: this is step 3 (after auth backend 100 and skeleton 110) — the UI needs machines to exist before images/containers/quadlets pages are usable, and this PRD proves the write path (forms, file upload, mutations) early.
