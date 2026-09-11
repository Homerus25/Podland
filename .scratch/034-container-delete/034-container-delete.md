# 034 - Implement Container Delete Endpoint

**Status**: completed

## Parent
030-container-list

## What to build
Implement the container delete endpoint that allows users to remove containers from a machine. This should handle both running and stopped containers with appropriate force options.

## Acceptance criteria
- [ ] ContainerService.Delete method added to remove a container by ID
- [ ] ContainerHandler.Delete implemented with proper parameter validation
- [ ] Support for query parameters like force=true to remove running containers
- [ ] Response returns 204 No Content on successful deletion
- [ ] Proper error handling for container not found, container already removed, and connection errors
- [ ] User authentication and authorization verified (user must have access to the machine)
- [ ] Container deletion uses the machine's connection context
- [ ] Unit tests for service and handler methods
- [ ] Integration test for the complete delete flow

## Blocked by
- 030-container-list (container infrastructure is in place)
- 032-container-get (get container by ID should work first)

## Notes
This implements DELETE /api/machines/{id}/containers/{containerId} endpoint. Should use podman ContainerRemove API with force option support.
