--- 
title: Backend Service Delete Method for Machine Deletion
parent: implement machine deletion
date: 2026-07-03
status: completed
labels:
  - machines
  - backend
  - deletion
  - service-layer
author: vibe-agent
---

## Parent

Part of the "implement machine deletion" feature.

## What to build

Add a `Delete` method to `MachineService` that provides the business logic for machine deletion. This method should:

- Accept userID and machineID as parameters
- Verify the machine exists and the user is the owner
- Orchestrate the deletion process including related data cleanup
- Return appropriate error types for different failure scenarios

The method signature should be:
```go
func (s *MachineService) Delete(ctx context.Context, userID, machineID string) error
```

## Acceptance criteria

- [ ] `Delete` method added to `MachineService` with proper signature
- [ ] Verifies user is owner of machine before deletion
- [ ] Returns `ErrMachineNotFound` when machine doesn't exist
- [ ] Returns `ErrAccessDenied` when user is not owner
- [ ] Returns appropriate error on database failures
- [ ] Method integrates with existing service interfaces
- [ ] Follows existing code style and patterns

## Blocked by

None - can start immediately

## Technical notes

- Reuse existing error types from `backend/internal/errors` package
- Use existing `GetMachineByID` for owner verification
- Follow the same context pattern as existing service methods
- The actual deletion and cleanup will be implemented in subsequent slices
