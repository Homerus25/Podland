--- 
title: Frontend Delete Button and Confirmation Dialog
parent: implement machine deletion
date: 2026-07-03
status: ready-for-agent
labels:
  - machines
  - frontend
  - deletion
  - ui
  - react
author: vibe-agent
---

## Parent

Part of the "implement machine deletion" feature.

## What to build

Create the user interface for machine deletion including delete buttons and confirmation dialog. The UI should follow the existing Chakra UI design patterns.

## Acceptance criteria

- [ ] Delete button added to machine list items
- [ ] Delete button added to machine detail view
- [ ] Reusable confirmation dialog component created
- [ ] Confirmation dialog shows machine name for verification
- [ ] Dialog has "Cancel" and "Delete" buttons
- [ ] Delete button is disabled for non-owners
- [ ] Loading state shown during deletion
- [ ] Success/error notifications displayed
- [ ] UI follows existing Chakra UI patterns and styling

## Blocked by

None - can start immediately

## Technical notes

- Create `DeleteMachineDialog` component in `frontend/src/features/machines/components/`
- Use Chakra UI's `AlertDialog` for confirmation
- Add delete buttons to `MachineCard` and `MachineDetailPage` components
- Show machine name in confirmation: "Are you sure you want to delete 'machine-name'?"
- Delete button should be red and prominent
- Disable delete button when deletion is in progress
- Access control: only show delete button for machine owners (admin permission)
- Use existing permission system from context