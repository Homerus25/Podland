# Podman Management API: Quadlet Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

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

### POST /api/machines/{id}/quadlets/auto-update

Run `podman auto-update` on the machine's quadlet-managed containers, streaming
the remote output live as NDJSON and terminating with a parsed report.

Only quadlets whose unit declares an update policy are visible to
`podman auto-update` — i.e. the `.container` file carries `AutoUpdate=registry`
(quadlet naming) and the unit is running. Policy-less quadlets are silently
skipped by podman.

**Permission level:** Write (read-level grantees receive 403 `ACCESS_DENIED`).

**Authentication:** Required

**Query Parameters:**
- `dryRun` (boolean, optional, default `false`): when `true`, passes
  `--dry-run` so podman checks for updates and reports what it *would* do
  without pulling or restarting anything. The report's `overall` is
  `dry-run` in that case.

**Response (Success - 200):** a live NDJSON stream (`Content-Type: application/json`),
one JSON event per line. Because registry pulls routinely exceed the global
write timeout, this endpoint extends its own response write deadline to
**10 minutes**; the stream stays open until the run finishes.

**Event schema** (all events share `operationId`, `machineId`, `timestamp`):

Log event — one line of remote `podman auto-update` output as it arrives:
```json
{
  "operationId": "uuid",
  "type": "log",
  "machineId": "machine uuid",
  "stream": "stdout|stderr",
  "line": "human-readable podman output line",
  "timestamp": "2026-08-30T10:00:00Z"
}
```

Complete event — the terminal event on success, always exactly one, carrying
the parsed report:
```json
{
  "operationId": "uuid",
  "type": "complete",
  "machineId": "machine uuid",
  "report": {
    "containers": [
      {
        "unit": "myapp.service",
        "containerName": "systemd-myapp",
        "imageName": "docker.io/library/myapp:latest",
        "updated": "updated|failed|pending|up-to-date",
        "error": "optional error text when updated is \"failed\""
      }
    ],
    "overall": "completed|completed-with-failures|dry-run|unparsable-report",
    "exitCode": 0
  },
  "timestamp": "2026-08-30T10:00:05Z"
}
```

Error event — the terminal event when the run fails before/instead of a
report (SSH failure, machine unreachable, or cancellation):
```json
{
  "operationId": "uuid",
  "type": "error",
  "machineId": "machine uuid",
  "error": {
    "code": "CONNECTION_FAILED|OPERATION_CANCELLED",
    "message": "human-readable message"
  },
  "timestamp": "2026-08-30T10:00:05Z"
}
```

**Cancellation semantics:** the run is bound to the request context — if the
client disconnects, the remote `podman auto-update` is aborted and the
machine connection released. (The terminal event for the aborted run is
written to the already-closed connection, so clients observe cancellation
simply as the stream ending.)

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 500: `INTERNAL_ERROR`, `STREAMING_NOT_SUPPORTED`
- 502: `CONNECTION_FAILED`
