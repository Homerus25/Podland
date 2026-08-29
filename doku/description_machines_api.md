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

### GET /api/machines/{id}/status

Perform a live status check of a machine. The machine is online when the SSH connection to the host succeeds AND the Podman service API answers its ping over that connection; anything else — dial failure, auth failure, missing socket, ping failure, or timeout — is reported as `offline`. There is no third server-side state. A cached per-machine connection is reused when present; a full SSH dial only happens when cold. The probe is bounded by the same timeout regime as the registration connection test (`MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS`, 10 seconds default). Any user with access may check status (read level suffices). Connection error details are never included in the response; they are logged server-side only, because SSH-level errors describe the owner's infrastructure and machines can be shared.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "status": "online",
  "checkedAt": "2026-08-29T12:00:00Z"
}
```

- `status`: `"online"` or `"offline"` (offline is a valid answer, not an error)
- `checkedAt`: RFC3339 UTC timestamp

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
