---
title: "Add Machine modal with validated form and SSH private key upload"
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

An "Add Machine" flow on the machines page. Clicking the button opens a modal containing a MachineForm (create mode) built with react-hook-form. The form covers the required fields (name, hostname/IP, port, SSH username) and optional fields (description, custom podman socket path), plus an SSH private key file input read client-side via the File API and sent in the request body.

Inline validation runs before submit: required-field messages and a numeric port range check. On successful creation the modal closes and the machines list refetches. On failure the error envelope (code + message) is rendered in the form. Submit button shows a pending state that prevents double submits.

This slice introduces the shared Modal component (none exists yet) that later slices (delete confirmation, share modal) reuse.

## Acceptance criteria

- [ ] "Add Machine" button on the machines page opens a modal with the form
- [ ] Required fields (name, hostname, SSH username, private key file) show inline validation errors when empty on submit
- [ ] Port field validates as numeric within a sensible range with an inline error
- [ ] Optional fields (description, podman socket path) are submittable empty
- [ ] SSH private key file input is present and required in create mode; its contents are read client-side and sent in the request body
- [ ] Successful creation closes the modal and the machines list refetches showing the new machine
- [ ] Failed creation renders the error envelope (code + message) inside the form
- [ ] Submit button shows a pending state during the request and prevents double submits
- [ ] MSW tests cover: validation messages, create success (modal closes, list refetches), create failure (envelope shown)

## Blocked by

None - can start immediately
