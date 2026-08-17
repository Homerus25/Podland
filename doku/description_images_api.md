# Podman Management API: Image Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

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
