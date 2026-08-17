# Podman Management API: Machine Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

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
