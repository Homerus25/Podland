# 033 - Implement Container Start and Stop Endpoints

**Status**: completed

## Parent
030-container-list

## What to build
Implement container lifecycle management endpoints for starting and stopping containers. These endpoints should control the container state on the target machine.

## Acceptance criteria
- [ ] ContainerService.Start method added to start a container by ID
- [ ] ContainerService.Stop method added to stop a container by ID
- [ ] ContainerHandler.Start implemented with proper parameter validation
- [ ] ContainerHandler.Stop implemented with proper parameter validation
- [ ] Both endpoints return appropriate status codes (200 for success, 204 for already in desired state)
- [ ] Proper error handling for container not found, invalid state transitions, and connection errors
- [ ] User authentication and authorization verified (user must have access to the machine)
- [ ] Container operations use the machine's connection context
- [ ] Unit tests for service and handler methods
- [ ] Integration tests for both start and stop flows

## Blocked by
- 030-container-list (container infrastructure is in place)
- 032-container-get (get container by ID should work first)

## Notes
This implements:
- POST /api/machines/{id}/containers/{containerId}/start
- POST /api/machines/{id}/containers/{containerId}/stop

Should use podman ContainerStart and ContainerStop APIs.
