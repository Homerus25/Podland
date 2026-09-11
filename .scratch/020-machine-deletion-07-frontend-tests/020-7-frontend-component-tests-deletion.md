--- 
title: Frontend Component Tests for Machine Deletion
parent: implement machine deletion
date: 2026-07-03
status: ready-for-agent
labels:
  - machines
  - frontend
  - deletion
  - testing
  - component-tests
author: vibe-agent
---

## Parent

Part of the "implement machine deletion" feature.

## What to build

Frontend tests for machine deletion components and API integration, ensuring the UI and data flow work correctly.

## Acceptance criteria

- [ ] DeleteMachineDialog component tests (opening, closing, confirmation, cancellation)
- [ ] Delete button visibility tests (owner vs non-owner)
- [ ] Delete button interaction tests (click handlers)
- [ ] useDeleteMachine hook tests with mock API
- [ ] Integration tests for delete flow in MachineCard
- [ ] Integration tests for delete flow in MachineDetailPage
- [ ] Error message display tests
- [ ] Loading state tests
- [ ] Success notification tests
- [ ] All tests pass with mock data

## Blocked by

- `/home/homerus/Projects/Podland/.scratch/020-machine-deletion-04-frontend-delete-ui/020-4-frontend-delete-button-and-dialog.md` (Slice 4)
- `/home/homerus/Projects/Podland/.scratch/020-machine-deletion-05-frontend-api/020-5-frontend-api-integration.md` (Slice 5)

## Technical notes

- Create test files in `frontend/src/features/machines/__tests__/`
- Use Jest and React Testing Library
- Mock React Query hooks for controlled testing
- Mock API calls to test without real backend
- Test user interactions (clicks, dialog dismissals)
- Test access control scenarios
- Test error scenarios with mocked error responses
- Follow existing test patterns in the codebase
- Use Chakra UI test utilities where available