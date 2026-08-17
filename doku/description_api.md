# Podman Management API: Contract and Communication

## Overview

This document defines the API contract between the frontend and backend for the Podman Management Web-App. It specifies all endpoints, request/response formats, error codes, and communication patterns.

Endpoint documentation is split by domain:

- [Auth](description_auth_api.md) - Login, logout, token status
- [Users](description_users_api.md) - User creation (admin-only)
- [Machines](description_machines_api.md) - Machine CRUD, access management, event stream (SSE)
- [Containers](description_containers_api.md) - Container CRUD, lifecycle, logs
- [Images](description_images_api.md) - Image listing, pull, removal
- [Quadlets](description_quadlets_api.md) - Quadlet CRUD, lifecycle, generate, import

Authentication uses JWT Bearer tokens; see the [Auth](description_auth_api.md) document for details.

---

## Base URL

```
http://localhost:8080/api
```

In production, the API is served behind a Caddy reverse proxy, which routes:
- `/api/*` to the backend service
- All other requests to the frontend static files

---

## Common Headers

### Request Headers
```
Content-Type: application/json (for JSON requests)
Accept: application/json
```

### Response Headers
```
Content-Type: application/json
X-Request-ID: <uuid> (for tracing)
```

### Rate Limit Headers (on authenticated requests)
```
X-RateLimit-Limit: <number>
X-RateLimit-Remaining: <number>
X-RateLimit-Reset: <seconds until reset>
```

### CORS Headers
```
Access-Control-Allow-Origin: <configured origins>
Access-Control-Allow-Credentials: true
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization
```

---

## Error Response Format

All error responses follow this structure:

```json
{
  "requestId": "req-<uuid>",
  "error": {
    "message": "Human-readable error message",
    "code": "ERROR_CODE"
  }
}
```

### HTTP Status Codes

| Status Code | Error Code Category | Description |
|-------------|---------------------|-------------|
| 400 | `VALIDATION_FAILED`, `INVALID_REQUEST`, `INVALID_SSH_KEY` | Bad request |
| 401 | `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`, `INVALID_CREDENTIALS` | Unauthorized |
| 403 | `ACCESS_DENIED`, `PERMISSION_DENIED` | Forbidden |
| 404 | `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`, `IMAGE_NOT_FOUND` | Not found |
| 409 | `MACHINE_ALREADY_EXISTS`, `CONTAINER_ALREADY_RUNNING` | Conflict |
| 429 | `RATE_LIMIT_EXCEEDED` | Too many requests |
| 500 | `INTERNAL_ERROR` | Internal server error |
| 502 | `SERVICE_UNAVAILABLE`, `CONNECTION_FAILED` | Bad gateway |
| 503 | `SERVICE_UNAVAILABLE` | Service unavailable |

### Error Code Reference

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `MACHINE_NOT_FOUND` | 404 | Machine does not exist or user lacks access |
| `MACHINE_ALREADY_EXISTS` | 409 | Machine name already exists for this user |
| `CONNECTION_FAILED` | 502 | Failed to connect to machine (during registration or operation) |
| `CONNECTION_TEST_TIMEOUT` | 400 | Connection test timed out during registration |
| `INVALID_CREDENTIALS` | 401 | Wrong username or password |
| `TOKEN_MISSING` | 401 | No token provided |
| `TOKEN_INVALID` | 401 | Malformed or tampered token |
| `TOKEN_EXPIRED` | 401 | Token has expired |
| `RATE_LIMIT_EXCEEDED` | 429 | User has exceeded rate limit |
| `ACCESS_DENIED` | 403 | User does not have access to this resource |
| `PERMISSION_DENIED` | 403 | User lacks required permission for action |
| `CONTAINER_NOT_FOUND` | 404 | Container does not exist |
| `CONTAINER_ALREADY_RUNNING` | 409 | Container cannot be started (already running) |
| `CONTAINER_NOT_RUNNING` | 400 | Container cannot be stopped (not running) |
| `IMAGE_NOT_FOUND` | 404 | Image does not exist |
| `IMAGE_PULL_FAILED` | 500 | Failed to pull image |
| `QUADLET_NOT_FOUND` | 404 | Quadlet does not exist |
| `QUADLET_ALREADY_EXISTS` | 409 | Quadlet with this name already exists |
| `QUADLET_RESOURCE_RUNNING` | 400 | Quadlet-managed resource is running |
| `QUADLET_ALREADY_RUNNING` | 400 | Quadlet is already running |
| `QUADLET_NOT_RUNNING` | 400 | Quadlet is not running |
| `VALIDATION_FAILED` | 400 | Request data failed validation |
| `INVALID_REQUEST` | 400 | Malformed request body |
| `INVALID_SSH_KEY` | 400 | Invalid SSH private key format |
| `INTERNAL_ERROR` | 500 | Generic internal error (details hidden) |
| `SERVICE_UNAVAILABLE` | 503 | Service temporarily unavailable |
| `NOT_IMPLEMENTED` | 501 | Endpoint not yet implemented |

---

## Examples

### Example: Full Machine Workflow

1. **User registers a machine:**
```bash
POST /api/machines
{
  "name": "production-server",
  "hostname": "prod.example.com",
  "sshUsername": "deploy",
  "sshPrivateKey": "-----BEGIN RSA PRIVATE KEY-----\n..."
}
```

2. **User lists their machines:**
```bash
GET /api/machines
```

3. **User pulls an image with progress:**
```bash
POST /api/machines/{machineId}/images/pull
{
  "image": "nginx:latest"
}
```

4. **User creates a container:**
```bash
POST /api/machines/{machineId}/containers
{
  "name": "web-server",
  "image": "nginx:latest",
  "portBindings": [{"hostPort": 8080, "containerPort": 80}]
}
```

5. **User starts the container:**
```bash
POST /api/machines/{machineId}/containers/{containerId}/start
```

6. **User subscribes to events:**
```bash
GET /api/machines/{machineId}/events
```

---

## WebSocket Alternative (Future)

For future consideration, WebSocket could be used instead of SSE for:
- Bidirectional communication (client can send commands through same connection)
- Better handling of connection state
- Reduced overhead for frequent small messages

Current implementation uses SSE as it is simpler and sufficient for the use case.

---

## Versioning

Currently, the API has no version prefix. For future backward compatibility:
- Consider adding `/api/v1/` prefix when API stabilizes
- Use deprecation headers for old endpoints
- Document breaking changes between versions

---

## Rate Limiting

- **Scope**: Per-user
- **Limit**: 60 requests per minute (configurable)
- **Response Headers**:
  ```
  X-RateLimit-Limit: 60
  X-RateLimit-Remaining: 45
  X-RateLimit-Reset: 15
  ```
- **Behavior**: Returns 429 with error code `RATE_LIMIT_EXCEEDED`

---

## Request Timeout

- **Default**: 30 seconds for regular requests
- **Connection test**: 10 seconds (configurable)
- **Streaming requests**: No timeout (handled by client)

---

## Compression

- **Request**: Not supported (content-length usually small)
- **Response**: Could be added in future with Accept-Encoding header support

---

## Documentation

This API documentation is the source of truth. Consider generating:
- OpenAPI/Swagger specification for interactive documentation
- Client SDKs in various languages

---

*Generated from comprehensive grilling sessions*
*Last updated: 2026-06-30*
