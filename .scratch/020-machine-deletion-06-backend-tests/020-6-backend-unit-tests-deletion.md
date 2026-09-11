--- 
title: Backend Unit Tests for Machine Deletion
parent: implement machine deletion
date: 2026-07-03
status: ready-for-agent
labels:
  - machines
  - backend
  - deletion
  - testing
  - unit-tests
author: vibe-agent
---

## Parent

Part of the "implement machine deletion" feature.

## What to build

Comprehensive unit tests for the backend machine deletion functionality, covering all layers from handler to repository.

## Acceptance criteria

- [x] Service delete method tested with valid machine ID
- [x] Service delete method tested with non-existent machine ID
- [x] Service delete method tested with non-owner user
- [x] Service delete method tested with database errors
- [x] Repository delete method tested
- [x] Machine access cleanup tested
- [x] Connection cache cleanup tested
- [x] Handler delete endpoint tested with mock service
- [x] All tests use mock dependencies
- [x] Code coverage > 85% for deletion-related code

## Blocked by

- `/home/homerus/Projects/Podland/.scratch/020-machine-deletion-01-backend-service-delete/020-1-backend-service-delete-method.md` (Slice 1)
- `/home/homerus/Projects/Podland/.scratch/020-machine-deletion-02-access-cleanup/020-2-machine-access-cleanup.md` (Slice 2)
- `/home/homerus/Projects/Podland/.scratch/020-machine-deletion-03-connection-cache/020-3-connection-cache-invalidation.md` (Slice 3)

## Technical notes

- Add tests to existing test files in `backend/internal/domain/machines/`
- Use existing mock utilities from `backend/internal/testutils/`
- Test both happy paths and error conditions
- Use table-driven tests for similar scenarios
- Test edge cases (empty IDs, concurrent deletions, etc.)
- Follow existing test patterns and style
- Mock database calls to test repository layer
- Mock dependencies for service layer tests
