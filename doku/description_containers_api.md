# Podman Management API: Container Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

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
