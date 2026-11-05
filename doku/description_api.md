# Podman Management API: Contract and Communication

## Overview

This document defines the API contract between the frontend and backend for the Podman Management Web-App. It specifies all endpoints, request/response formats, error codes, and communication patterns.

Endpoint documentation is split by domain:

- [Auth](description_auth_api.md) - Login, logout, token status
- [Users](description_users_api.md) - User creation (admin-only)
- [Hosts](description_hosts_api.md) - Host CRUD, access management, event stream (SSE)
- [Containers](description_containers_api.md) - Container CRUD, lifecycle, logs
- [Images](description_images_api.md) - Image listing, pull, removal
- [Networks](description_networks_api.md) - Network listing (read-only)
- [Pods](description_pods_api.md) - Pod list, create, lifecycle, prune, inspect
- [Quadlets](description_quadlets_api.md) - Quadlet CRUD, lifecycle, generate, import
- [Secrets](description_secrets_api.md) - Secret list/inspect/create/delete with admin-only value reveal

Authentication uses JWT Bearer tokens; see the [Auth](description_auth_api.md) document for details.

---

## Base URL

```
http://localhost:8080/api
```

In production, one container serves everything on :8080: the backend handles
`/api/*`, and every other path is answered with the embedded frontend
(SPA fallback to `index.html`). During development, the Vite dev server on
:3000 proxies `/api` to the backend on :8080.

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

The origin decision depends on `ALLOWED_ORIGINS`; credentials are only ever emitted together with a specific exactly-matched configured origin:

- **Unset (default):** same-origin only — responses carry neither `Access-Control-Allow-Origin` nor `Access-Control-Allow-Credentials`.
- **`*`:** `Access-Control-Allow-Origin: *` (spec-valid wildcard, no credentials).
- **Configured origins:** an exactly-matching `Origin` gets `Access-Control-Allow-Origin: <origin>`, `Access-Control-Allow-Credentials: true`, and `Vary: Origin`; any other origin gets neither header.

Preflights (`OPTIONS`) answer `204` with the same origin decision plus:
```
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, PATCH, OPTIONS
Access-Control-Allow-Headers: Origin, Content-Type, Accept, Authorization, Cookie
Access-Control-Max-Age: 86400
```

Actual requests also carry:
```
Access-Control-Allow-Headers: Origin, Content-Type, Accept, Authorization, Cookie
Access-Control-Expose-Headers: Request-Id, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset
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
| 400 | `VALIDATION_FAILED`, `INVALID_REQUEST`, `INVALID_SSH_KEY`, `CONTAINER_ALREADY_RUNNING` | Bad request |
| 401 | `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`, `TOKEN_REVOKED`, `INVALID_CREDENTIALS` | Unauthorized |
| 403 | `ACCESS_DENIED`, `PERMISSION_DENIED` | Forbidden |
| 404 | `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`, `IMAGE_NOT_FOUND` | Not found |
| 409 | `HOST_ALREADY_EXISTS` | Conflict |
| 429 | `RATE_LIMIT_EXCEEDED` | Too many requests |
| 500 | `INTERNAL_ERROR` | Internal server error |
| 502 | `SERVICE_UNAVAILABLE`, `CONNECTION_FAILED` | Bad gateway |
| 503 | `SERVICE_UNAVAILABLE` | Service unavailable |

### Error Code Reference

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `HOST_NOT_FOUND` | 404 | Host does not exist or user lacks access |
| `HOST_ALREADY_EXISTS` | 409 | Host name already exists for this user |
| `CONNECTION_FAILED` | 502 | Failed to connect to host (during registration or operation) |
| `CONNECTION_TEST_TIMEOUT` | 400 | Connection test timed out during registration |
| `HOST_NOT_ALLOWED` | 400 | Host hostname rejected by the instance's opt-in egress block (`HOST_BLOCK_PRIVATE_RANGES`): blocked address range or unresolvable while enabled |
| `INVALID_CREDENTIALS` | 401 | Wrong username or password |
| `TOKEN_MISSING` | 401 | No token provided |
| `TOKEN_INVALID` | 401 | Malformed or tampered token |
| `TOKEN_EXPIRED` | 401 | Token has expired |
| `TOKEN_REVOKED` | 401 | Token has been revoked (logout, password change, admin password reset) |
| `RATE_LIMIT_EXCEEDED` | 429 | User has exceeded rate limit |
| `ACCESS_DENIED` | 403 | User does not have access to this resource |
| `PERMISSION_DENIED` | 403 | User lacks required permission for action |
| `CONTAINER_NOT_FOUND` | 404 | Container does not exist |
| `CONTAINER_ALREADY_RUNNING` | 400 | Container cannot be started (already running) |
| `CONTAINER_NOT_RUNNING` | 400 | Container cannot be stopped (not running) |
| `IMAGE_NOT_FOUND` | 404 | Image does not exist |
| `IMAGE_PULL_FAILED` | 500 | Failed to pull image |
| `QUADLET_NOT_FOUND` | 404 | Quadlet does not exist |
| `QUADLET_ALREADY_EXISTS` | 409 | Quadlet with this name already exists |
| `QUADLET_RESOURCE_RUNNING` | 400 | Quadlet-managed resource is running |
| `QUADLET_ALREADY_RUNNING` | 400 | Quadlet is already running |
| `QUADLET_NOT_RUNNING` | 400 | Quadlet is not running |
| `SECRET_NOT_FOUND` | 404 | Secret does not exist |
| `SECRET_ALREADY_EXISTS` | 409 | Secret with this name already exists |
| `SECRET_IN_USE` | 409 | Secret is in use by one or more containers |
| `SECRET_CREATION_FAILED` | 500 | Failed to create secret |
| `VALIDATION_FAILED` | 400 | Request data failed validation |
| `INVALID_REQUEST` | 400 | Malformed request body |
| `INVALID_SSH_KEY` | 400 | Invalid SSH private key format |
| `INTERNAL_ERROR` | 500 | Generic internal error (details hidden) |
| `SERVICE_UNAVAILABLE` | 503 | Service temporarily unavailable |
| `NOT_IMPLEMENTED` | 501 | Endpoint not yet implemented |

### Host Permission Levels

Access to a host is granted at one of three levels: `read` < `write` < `admin` (owners implicitly hold `admin`). Every endpoint under `/api/hosts/{id}` requires at least `read`. Mutating endpoints — container create/delete/start/stop, image pull/delete, quadlet create/import/update/delete/start/stop/restart, secret create — additionally require `write` or above and answer 403 `ACCESS_DENIED` for lower levels. Read-only endpoints (lists, gets, logs, quadlet generate, events, secret metadata list/inspect) remain available at `read`. Secret value reveal (`GET .../secrets/{secretId}?reveal=true`) is the one `admin`-only operation and answers 403 `ACCESS_DENIED` below admin.

---

## Examples

### Example: Full Host Workflow

1. **User registers a host:**
```bash
POST /api/hosts
{
  "name": "production-server",
  "hostname": "prod.example.com",
  "sshUsername": "deploy",
  "sshPrivateKey": "-----BEGIN RSA PRIVATE KEY-----\n..."
}
```

2. **User lists their hosts:**
```bash
GET /api/hosts
```

3. **User pulls an image with progress:**
```bash
POST /api/hosts/{hostId}/images/pull
{
  "image": "nginx:latest"
}
```

4. **User creates a container:**
```bash
POST /api/hosts/{hostId}/containers
{
  "name": "web-server",
  "image": "nginx:latest",
  "portBindings": [{"hostPort": 8080, "containerPort": 80}]
}
```

5. **User starts the container:**
```bash
POST /api/hosts/{hostId}/containers/{containerId}/start
```

6. **User subscribes to events:**
```bash
GET /api/hosts/{hostId}/events
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

## Request Body Limits

- **JSON endpoints (global)**: 10 MiB cap on the request body, enforced centrally in the API's JSON decode layer. A body over the cap answers 400 `INVALID_REQUEST` with message "Invalid request body" and is cut off at the reader, never fully buffered.
- **Compose convert** (`POST /api/quadlets/convert-compose`): tighter 1 MiB cap on the raw request body (same 400 `INVALID_REQUEST` answer when exceeded).
- **Volume populate** (multipart upload): capped by `VOLUME_UPLOAD_MAX_BYTES` (default 2 GiB); exceeding it answers 413 `VOLUME_UPLOAD_TOO_LARGE`.

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
