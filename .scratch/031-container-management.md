# 031 - Container Management Endpoints

**Status**: completed

## What to build
Implement the remaining container management endpoints that are currently returning "Not implemented" (501) errors. This includes Create, Get, Start, Stop, and Delete operations for containers.

## Child Issues
- [031-container-create](031-container-create/031-container-create.md) - POST /api/machines/{id}/containers
- [032-container-get](032-container-get/032-container-get.md) - GET /api/machines/{id}/containers/{containerId}
- [033-container-start-stop](033-container-start-stop/033-container-start-stop.md) - POST start/stop actions
- [034-container-delete](034-container-delete/034-container-delete.md) - DELETE /api/machines/{id}/containers/{containerId}

## Background
Currently, only the List endpoint for containers is implemented (issue 030-container-list). The following handler methods in `/backend/internal/domain/containers/handler.go` return 501 Not Implemented:
- `Create` (line 144-146)
- `Get` (line 149-151) 
- `Start` (line 154-156)
- `Stop` (line 159-161)
- `Delete` (line 164-166)

These endpoints are already routed in `main.go` (lines 147-153) but need implementation.

## Dependencies
- All container management endpoints require the connection infrastructure from 030-container-list
- Get, Start, Stop, and Delete endpoints may benefit from the Get by ID functionality, creating a dependency: 032-container-get → 033-container-start-stop, 034-container-delete

## Suggested Order
1. 031-container-create (independent of other container ops)
2. 032-container-get (foundation for other single-container operations)
3. 033-container-start-stop (depends on get by ID)
4. 034-container-delete (depends on get by ID)

## Notes
All implementations should follow the existing patterns:
- Proper authentication/authorization using JWT middleware
- Connection management via the ConnectionManager
- Error handling using the errors package
- Request/response DTOs in the dto.go file
- Comprehensive unit and integration tests
