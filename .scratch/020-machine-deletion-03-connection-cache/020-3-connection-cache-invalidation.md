--- 
title: Connection Cache Invalidation on Machine Deletion
parent: implement machine deletion
date: 2026-07-03
status: ready-for-agent
labels:
  - machines
  - backend
  - deletion
  - connections
  - cache
  - security
author: vibe-agent
---

## Parent

Part of the "implement machine deletion" feature.

## What to build

Add functionality to invalidate and remove connections from the connection cache when a machine is deleted. This prevents orphaned SSH connections and potential security issues.

## Acceptance criteria

- [ ] `ConnectionCache` has method to remove connections by machine ID
- [ ] `ConnectionManager` exposes method to invalidate machine connections
- [ ] Machine deletion flow calls connection cleanup
- [ ] Active connections for deleted machines are closed/properly cleaned up
- [ ] No memory leaks from orphaned connection entries

## Blocked by

- `/home/homerus/Projects/Podland/.scratch/020-machine-deletion-01-backend-service-delete/020-1-backend-service-delete-method.md` (Slice 1)

## Technical notes

- Add `RemoveMachine(machineID string)` method to `ConnectionCache`
- Consider closing active connections gracefully
- Add corresponding method to `ConnectionManager` if needed for proper integration
- Call from `MachineService.Delete` after database cleanup
- Update connection manager interface if using dependency injection
- Handle race conditions where connections are being created while deletion occurs

## Outcome

Superseded 2026-09-10 by machine lifecycle epic 300, slice 300-5 (full
teardown wiring in machine deletion): `connections.Manager.RemoveMachine`
already exists and is graceful (refcount + dead marker, in-flight calls
finish first), and `MachineService.Delete` now calls it after a successful
DB delete, followed by the SSH-exec pool's invalidation over the
machines-local `sshInvalidator` seam. The create-while-deleting race is
handled inside the connection manager's removed-during-acquisition error.