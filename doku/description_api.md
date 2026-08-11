# Podman Management API: Contract and Communication

## Overview

This document defines the API contract between the frontend and backend for the Podman Management Web-App. It specifies all endpoints, request/response formats, error codes, and communication patterns.

---

## Base URL

```
http://localhost:8080/api
```

In production, the API is served behind a Caddy reverse proxy, which routes:
- `/api/*` to the backend service
- All other requests to the frontend static files

---

## Authentication

### Mechanism
- **JWT tokens** signed with RS256
- **Storage**: HTTP-only cookies (named `token`)
- **Token expiration**: Configurable, default 60 minutes

### Cookie Details
```
Name: token
Path: /
HttpOnly: true
Secure: true (in production)
SameSite: Strict
Expires: <calculated from JWT expiration>
```

### Auth Endpoints

#### POST /api/auth/login

**Request:**
```json
Content-Type: application/json

{
  "username": "string",
  "password": "string"
}
```

**Response (Success - 200):**
- Sets HTTP-only cookie with JWT token
- Returns user information
```json
{
  "user": {
    "id": "string (uuid)",
    "username": "string"
  }
}
```

**Response (Error):**
- 401 Unauthorized: Invalid credentials (`INVALID_CREDENTIALS`)
- 400 Bad Request: Invalid request format

---

#### POST /api/auth/logout

**Request:**
- No body required
- Clears the `token` cookie

**Response (Success - 200):**
```json
{
  "message": "Logged out successfully"
}
```

---

#### GET /api/auth/status

**Request:**
- Sends `token` cookie automatically

**Response (Success - 200):**
```json
{
  "user": {
    "id": "string (uuid)",
    "username": "string",
    "createdAt": "ISO8601 timestamp"
  }
}
```

**Response (Error):**
- 401 Unauthorized: Invalid or missing token (`TOKEN_MISSING` or `TOKEN_INVALID`)

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

## Machine Endpoints

### POST /api/machines

Register a new remote machine.

**Authentication:** Required

**Request:**
```json
{
  "name": "string (required)",
  "description": "string (optional)",
  "hostname": "string (required)",
  "port": "integer (optional, default: 22)",
  "sshUsername": "string (required)",
  "sshPrivateKey": "string (required, PEM format)",
  "podmanSocketPath": "string (optional, default: '/run/podman/podman.sock')"
}
```

**Validation:**
- `name` must be unique per user
- `hostname` must be valid (DNS or IP)
- `sshPrivateKey` must be valid PEM format
- Backend will **test the SSH connection** before saving

**Response (Success - 201):**
```json
{
  "id": "string (uuid)",
  "name": "string",
  "description": "string",
  "hostname": "string",
  "port": 22,
  "sshUsername": "string",
  "sshKeyFingerprint": "string",
  "podmanSocketPath": "/run/podman/podman.sock",
  "ownerId": "string (uuid)",
  "createdAt": "ISO8601 timestamp",
  "updatedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `VALIDATION_FAILED`, `INVALID_SSH_KEY`, `CONNECTION_FAILED`, `CONNECTION_TEST_TIMEOUT`, `MACHINE_ALREADY_EXISTS`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 500: `INTERNAL_ERROR`

---

### GET /api/machines

List all machines the user has access to.

**Authentication:** Required

**Query Parameters:**
- None

**Response (Success - 200):**
```json
[
  {
    "id": "string (uuid)",
    "name": "string",
    "description": "string",
    "hostname": "string",
    "port": 22,
    "sshUsername": "string",
    "sshKeyFingerprint": "string",
    "podmanSocketPath": "/run/podman/podman.sock",
    "ownerId": "string (uuid)",
    "permission": "read|write|admin",
    "createdAt": "ISO8601 timestamp",
    "updatedAt": "ISO8601 timestamp"
  }
]
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 500: `INTERNAL_ERROR`

---

### GET /api/machines/{id}

Get details of a specific machine.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string (uuid)",
  "name": "string",
  "description": "string",
  "hostname": "string",
  "port": 22,
  "sshUsername": "string",
  "sshKeyFingerprint": "string",
  "podmanSocketPath": "/run/podman/podman.sock",
  "ownerId": "string (uuid)",
  "owner": {
    "id": "string",
    "username": "string"
  },
  "mission": "read|write|admin",
  "createdAt": "ISO8601 timestamp",
  "updatedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### PUT /api/machines/{id}

Update an existing machine's connection details.

**Authentication:** Required

**Request:**
```json
{
  "name": "string (optional)",
  "description": "string (optional)",
  "hostname": "string (optional)",
  "port": "integer (optional)",
  "sshUsername": "string (optional)",
  "sshPrivateKey": "string (optional, PEM format)",
  "podmanSocketPath": "string (optional)"
}
```

**Note:** If `sshPrivateKey` is provided, the backend will test the new connection.

**Response (Success - 200):**
- Same as GET /api/machines/{id}

**Response (Error):**
- 400: `VALIDATION_FAILED`, `INVALID_SSH_KEY`, `CONNECTION_FAILED`, `CONNECTION_TEST_TIMEOUT`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### DELETE /api/machines/{id}

Remove a machine.

**Authentication:** Required

**Response (Success - 204):**
- No body

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### GET /api/machines/{id}/access

List users with access to a machine.

**Authentication:** Required

**Response (Success - 200):**
```json
[
  {
    "userId": "string (uuid)",
    "username": "string",
    "permission": "read|write|admin",
    "grantedAt": "ISO8601 timestamp"
  }
]
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### POST /api/machines/{id}/access

Grant another user access to a machine.

**Authentication:** Required (must be owner or have admin permission)

**Request:**
```json
{
  "userId": "string (uuid, required)",
  "permission": "read|write|admin (required)"
}
```

**Response (Success - 201):**
```json
{
  "machineId": "string (uuid)",
  "userId": "string (uuid)",
  "permission": "read|write|admin"
}
```

**Response (Error):**
- 400: `VALIDATION_FAILED`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `USER_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### DELETE /api/machines/{id}/access/{userId}

Revoke a user's access to a machine.

**Authentication:** Required (must be owner or have admin permission)

**Response (Success - 204):**
- No body

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `USER_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

## Container Endpoints

### GET /api/machines/{id}/containers

List all containers on a machine.

**Authentication:** Required

**Query Parameters:**
- `all` (boolean, optional): Include stopped containers (default: false, only running)
- `filters` (object, optional): Filter by state, name, etc.

**Response (Success - 200):**
```json
[
  {
    "id": "string",
    "name": "string",
    "state": "created|running|paused|stopped|removing|exited|dead",
    "image": "string",
    "imageID": "string",
    "command": "string[]",
    "createdAt": "ISO8601 timestamp",
    "portBindings": [
      {
        "hostPort": integer,
        "containerPort": integer,
        "protocol": "tcp|udp"
      }
    ],
    "mounts": [
      {
        "source": "string",
        "destination": "string",
        "mode": "ro|rw"
      }
    ],
    "environment": { "key": "value" },
    "labels": { "key": "value" }
  }
]
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/containers

Create a new container on a machine.

**Authentication:** Required

**Request:**
```json
{
  "name": "string (optional)",
  "image": "string (required)",
  "command": "string[] (optional)",
  "args": "string[] (optional)",
  "workingDir": "string (optional)",
  "environment": { "key": "value" } (optional),
  "labels": { "key": "value" } (optional),
  "portBindings": [
    {
      "hostPort": integer (required),
      "containerPort": integer (required),
      "protocol": "tcp|udp" (optional, default: tcp)
    }
  ] (optional),
  "mounts": [
    {
      "source": "string (required)",
      "destination": "string (required)",
      "mode": "ro|rw" (optional, default: rw)
    }
  ] (optional),
  "privileged": boolean (optional, default: false),
  "restartPolicy": "no|always|unless-stopped|on-failure" (optional, default: no),
  "memory": "string (optional, e.g., '512m')",
  "cpus": number (optional)
}
```

**Response (Success - 201):**
```json
{
  "id": "string",
  "name": "string",
  "state": "created|running",
  "image": "string",
  "imageID": "string",
  "createdAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `VALIDATION_FAILED`, `INVALID_REQUEST`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `IMAGE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/machines/{id}/containers/{containerId}

Get details of a specific container.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string",
  "name": "string",
  "state": "created|running|paused|stopped|removing|exited|dead",
  "image": "string",
  "imageID": "string",
  "command": "string[]",
  "createdAt": "ISO8601 timestamp",
  "startedAt": "ISO8601 timestamp (optional)",
  "finishedAt": "ISO8601 timestamp (optional)",
  "exitCode": integer (optional),
  "portBindings": [
    {
      "hostPort": integer,
      "containerPort": integer,
      "protocol": "tcp|udp"
    }
  ],
  "mounts": [
    {
      "source": "string",
      "destination": "string",
      "mode": "ro|rw"
    }
  ],
  "environment": { "key": "value" },
  "labels": { "key": "value" },
  "networks": [
    {
      "name": "string",
      "ipAddress": "string",
      "macAddress": "string"
    }
  ],
  "logPath": "string",
  "config": { ... }
}
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### POST /api/machines/{id}/containers/{containerId}/start

Start a stopped container.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string",
  "state": "running",
  "startedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `CONTAINER_ALREADY_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/containers/{containerId}/stop

Stop a running container.

**Authentication:** Required

**Request:**
```json
{
  "force": boolean (optional, default: false),
  "timeout": integer (optional, seconds, default: 10)
}
```

**Response (Success - 200):**
```json
{
  "id": "string",
  "state": "stopped",
  "finishedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `CONTAINER_NOT_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### DELETE /api/machines/{id}/containers/{containerId}

Remove a container.

**Authentication:** Required

**Query Parameters:**
- `force` (boolean, optional): Force removal of running container
- `volumes` (boolean, optional): Remove associated volumes

**Response (Success - 204):**
- No body

**Response (Error):**
- 400: `CONTAINER retroced` (if running and force not set)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

## Image Endpoints

### GET /api/machines/{id}/images

List all images on a machine.

**Authentication:** Required

**Query Parameters:**
- `filters` (object, optional): Filter by name, label, etc.

**Response (Success - 200):**
```json
[
  {
    "id": "string",
    "repoTags": ["string"],
    "repoDigests": ["string"],
    "parentID": "string (optional)",
    "comment": "string (optional)",
    "createdAt": "ISO8601 timestamp",
    "size": integer,
    "os": "string (optional)",
    "architecture": "string (optional)",
    "labels": { "key": "value" } (optional)
  }
]
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/images/pull

Pull an image from a registry.

**Authentication:** Required

**Request:**
```json
{
  "image": "string (required)",
  "credentials": {
    "username": "string (optional)",
    "password": "string (optional)"
  } (optional),
  "platform": "string (optional, e.g., 'linux/amd64')"
}
```

**Response (Streaming - 200):**
- **Content-Type**: `application/json`
- **Format**: Newline-delimited JSON (NDJSON)
- Each line is a JSON object with the following structure:

```json
{
  "operationId": "string (uuid)",
  "type": "progress|complete|error",
  "machineId": "string (uuid)",
  "state": {
    "current": integer,
    "total": integer,
    "status": "string",
    "message": "string"
  } (only for type: progress),
  "timestamp": "ISO8601 timestamp"
}
```

For type: `error`:
```json
{
  "operationId": "string (uuid)",
  "type": "error",
  "error": {
    "code": "string",
    "message": "string"
  },
  "timestamp": "ISO8601 timestamp"
}
```

For type: `complete`:
```json
{
  "operationId": "string (uuid)",
  "type": "complete",
  "machineId": "string (uuid)",
  "state": {
    "status": "completed"
  },
  "timestamp": "ISO8601 timestamp"
}
```

**Aborting:**
- Client can abort the request by closing the connection
- Backend will stop the pull operation and clean up
- Final event will be of type `error` with code `OPERATION_CANCELLED`

**Response (Error):**
- 400: `VALIDATION_FAILED`, `INVALID_REQUEST`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### DELETE /api/machines/{id}/images/{imageId}

Remove an image from a machine.

**Authentication:** Required

**Query Parameters:**
- `force` (boolean, optional): Force removal even if used by containers

**Response (Success - 204):**
- No body

**Response (Error):**
- 400: `IMAGE_USED_BY_CONTAINERS` (if force not set)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `IMAGE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

## Quadlet Endpoints

### GET /api/machines/{id}/quadlets

List all quadlets (Podman systemd unit files) on a machine.

**Authentication:** Required

**Query Parameters:**
- `resource` (string, optional): Filter by resource type (container, pod, image, volume, network)
- `name` (string, optional): Filter by quadlet name

**Response (Success - 200):**
```json
[
  {
    "id": "string (uuid)",
    "name": "string",
    "path": "string",
    "unit": "string",
    "resource": "container|pod|image|volume|network",
    "state": "unknown|stopped|running|error",
    "raw": "string (optional, the raw quadlet file content)"
  }
]
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/machines/{id}/quadlets/{quadletId}

Get details of a specific quadlet including its content.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string (uuid)",
  "name": "string",
  "path": "string",
  "unit": "string",
  "resource": "container|pod|image|volume|network",
  "state": "unknown|stopped|running|error",
  "content": "string",
  "generation": integer,
  "createdAt": "ISO8601 timestamp",
  "updatedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/quadlets

Create a new quadlet on a machine.

**Authentication:** Required

**Request:**
```json
{
  "name": "string (required)",
  "resource": "container|pod|image|volume|network (required)",
  "content": "string (required, the quadlet file content)"
}
```

**Response (Success - 201):**
```json
{
  "id": "string (uuid)",
  "name": "string",
  "path": "string",
  "unit": "string",
  "resource": "container|pod|image|volume|network",
  "state": "unknown|stopped",
  "content": "string",
  "createdAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `VALIDATION_FAILED`, `INVALID_REQUEST`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 409: `QUADLET_ALREADY_EXISTS`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### PUT /api/machines/{id}/quadlets/{quadletId}

Update an existing quadlet's content.

**Authentication:** Required

**Request:**
```json
{
  "content": "string (required, the updated quadlet file content)"
}
```

**Response (Success - 200):**
- Same as GET /api/machines/{id}/quadlets/{quadletId}

**Response (Error):**
- 400: `VALIDATION_FAILED`, `INVALID_REQUEST`
- 401: `TOKEN_MISSING`, `TOKEN_AUTO_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### DELETE /api/machines/{id}/quadlets/{quadletId}

Remove a quadlet from a machine.

**Authentication:** Required

**Query Parameters:**
- `stop` (boolean, optional): Stop the resource before removing (default: true)
- `force` (boolean, optional): Force remove even if resource is running

**Response (Success - 204):**
- No body

**Response (Error):**
- 400: `QUADLET_RESOURCE_RUNNING` (if running and force/stop not set)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/quadlets/{quadletId}/start

Start a stopped quadlet-managed resource.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string",
  "state": "running",
  "unit": "string"
}
```

**Response (Error):**
- 400: `QUADLET_ALREADY_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/quadlets/{quadletId}/stop

Stop a running quadlet-managed resource.

**Authentication:** Required

**Query Parameters:**
- `force` (boolean, optional): Force stop
- `timeout` (integer, optional): Stop timeout in seconds

**Response (Success - 200):**
```json
{
  "id": "string",
  "state": "stopped",
  "unit": "string"
}
```

**Response (Error):**
- 400: `QUADLET_NOT_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/quadlets/{quadletId}/restart

Restart a quadlet-managed resource.

**Authentication:** Required

**Query Parameters:**
- `force` (boolean, optional): Force restart
- `timeout` (integer, optional): Stop timeout in seconds before restart

**Response (Success - 200):**
```json
{
  "id": "string",
  "state": "running",
  "unit": "string",
  "restartedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `QUADLET_NOT_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/machines/{id}/quadlets/{quadletId}/logs

Get logs for a quadlet-managed resource.

**Authentication:** Required

**Query Parameters:**
- `follow` (boolean, optional): Follow log output
- `since` (string, optional): Show logs since timestamp (ISO8601)
- `tail` (integer, optional): Number of lines to show from the end

**Response (Success - 200):**
```json
{
  "id": "string",
  "logs": "string (multiline log content)"
}
```

For streaming logs:
- **Content-Type**: `application/json`
- **Format**: Newline-delimited JSON (NDJSON) for streaming
- Each line is a log line object:

```json
{
  "timestamp": "ISO8601 timestamp",
  "level": "string (optional, e.g., INFO, ERROR, WARN)",
  "message": "string"
}
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/machines/{id}/containers/{containerId}/logs

Get logs for a container.

**Authentication:** Required

**Query Parameters:**
- `follow` (boolean, optional): Follow log output (accepted but not implemented in this slice)
- `since` (string, optional): Show logs since timestamp (ISO8601)
- `tail` (integer, optional): Number of lines to show from the end

**Response (Success - 200):**
```json
{
  "id": "string",
  "logs": "string (multiline log content)"
}
```

**Response (Error):**
- 400: `INVALID_SINCE_PARAM`, `INVALID_TAIL_PARAM`, `MISSING_MACHINE_ID`, `MISSING_CONTAINER_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### POST /api/machines/{id}/quadlets/{quadletId}/generate

Generate a quadlet file from an existing resource (container or pod).

**Authentication:** Required

**Request:**
```json
{
  "resourceId": "string (required)",
  "resourceType": "container|pod (required)"
}
```

**Response (Success - 200):**
```json
{
  "id": "string (uuid)",
  "name": "string",
  "path": "string",
  "unit": "string",
  "resource": "container|pod",
  "state": "unknown",
  "content": "string (generated quadlet content)",
  "createdAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `INVALID_REQUEST`, `RESOURCE_NOT_FOUND`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`, `POD_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/quadlets/import

Import quadlets from uploaded content.

**Authentication:** Required

**Request:**
```json
{
  "quadlets": [
    {
      "name": "string (required, quadlet name without extension)",
      "resource": "container|pod|image|volume|network (required)",
      "content": "string (required, quadlet file content)"
    }
  ]
}
```

**Response (Success - 201):**
```json
{
  "imported": [
    {
      "id": "string (uuid)",
      "name": "string",
      "path": "string",
      "state": "unknown|stopped|running|error"
    }
  ],
  "failed": [
    {
      "name": "string",
      "error": "string"
    }
  ]
}
```

**Response (Error):**
- 400: `VALIDATION_FAILED`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

## Event Endpoints (SSE)

### GET /api/machines/{id}/events

Subscribe to Podman events for a specific machine.

**Authentication:** Required

**Response (Success - 200):**
- **Content-Type**: `text/event-stream`
- **Connection**: `keep-alive`
- **Cache-Control**: `no-cache`

**Event Format:**
```
event: <event type>
data: <JSON payload>

```

**Event Types:**
- `container`: Container lifecycle events (start, stop, create, remove, die, etc.)
- `image`: Image events (pull, push, remove, build, etc.)
- `volume`: Volume events
- `network`: Network events
- `pod`: Pod events
- `build`: Build events
- `error`: Error events

**Example Event:**
```
event: container
data: {"status":"start","id":"abc123","name":"my-container","from":"nginx:latest"}

```

**Reconnection:**
- Client should automatically reconnect if connection is lost
- Backend will maintain the connection as long as there are active subscribers
- When last client disconnects, backend will close the connection to Podman Service API

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

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
