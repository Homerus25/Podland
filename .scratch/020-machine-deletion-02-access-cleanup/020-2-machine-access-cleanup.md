--- 
title: Machine Access Cleanup on Deletion
parent: implement machine deletion
date: 2026-07-03
status: ready-for-agent
labels:
  - machines
  - backend
  - deletion
  - database
  - data-cleanup
author: vibe-agent
---

## Parent

Part of the "implement machine deletion" feature.

## What to build

Extend the machine deletion functionality to clean up related `machine_access` entries when a machine is deleted. This prevents orphaned permission entries in the database.

## Acceptance criteria

- [ ] Machine deletion removes all associated `machine_access` entries from database
- [ ] Cleanup happens atomically with machine deletion (transaction)
- [ ] No orphaned access entries remain after machine deletion
- [ ] Handles database errors during cleanup appropriately
- [ ] Maintains referential integrity

## Blocked by

- `/home/homerus/Projects/Podland/.scratch/020-machine-deletion-01-backend-service-delete/020-1-backend-service-delete-method.md` (Slice 1)

## Technical notes

- Add `DeleteByMachine(machineID string) error` method to `MachineAccessRepository`
- Call this method from the `MachineService.Delete` method
- Consider using database transactions for atomic operations
- Update repository interface in `models/machine_access.go`
- Test cleanup with machines that have multiple access entries

## Outcome

Superseded 2026-09-10 by machine lifecycle epic 300 (design decision 9,
slice 300-5): the schema already cascades `machine_access` rows on machine
deletion (`ON DELETE CASCADE`, foreign keys on), so no explicit
`DeleteByMachine` cleanup, no transaction, and no wider repository interface
were added. The delete path documents the cascade reliance in
`backend/internal/domain/machines/service.go`.