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
