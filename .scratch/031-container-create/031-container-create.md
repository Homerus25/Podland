# 031 - Implement Container Create Endpoint

**Status**: completed

## Parent
030-container-list

## What to build
Implement the container create endpoint that allows users to create new containers on a specific machine. This endpoint should accept container configuration via JSON request body and create the container using the Podman API.

## Acceptance criteria
- [ ] ContainerService.Get method added to fetch a specific container by ID
- [ ] ContainerHandler.Create implemented with proper request validation
- [ ] Request DTO created for container creation (name, image, command, env vars, ports, volumes, etc.)
- [ ] Response returns created container information with 201 status code
- [ ] Proper error handling for invalid requests, missing parameters, and Podman API errors
- [ ] User authentication and authorization verified (user must have access to the machine)
- [ ] Container creation uses the machine's connection context
- [ ] Unit tests for service and handler methods
- [ ] Integration test for the complete create flow

## Blocked by
- 030-container-list (container infrastructure is in place)

## Notes
This implements POST /api/machines/{id}/containers endpoint. The container create operation should use the podman ContainerCreate API with appropriate container configuration options.
