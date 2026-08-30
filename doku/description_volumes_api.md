# Podman Management API: Volume Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

---

## Volume Endpoints

### GET /api/machines/{id}/volumes

List every volume on a machine, named and anonymous alike.

**Authentication:** Required

**Access:** Any user with a machine-access row for `{id}` (read level).

**Response (Success - 200):**
```json
[
  {
    "name": "string",
    "driver": "string",
    "createdAt": "ISO8601 timestamp",
    "anonymous": boolean,
    "labels": { "key": "value" },
    "options": { "key": "value" },
    "mountpoint": "string",
    "size": integer
  }
]
```

Notes:
- `size` is only present when podman reports usage for the volume.
- `labels` and `options` are always present; they are `{}` when empty.
- `createdAt` is an RFC3339 timestamp; podman does not store creation time for some very old volumes, in which case the zero timestamp is reported.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR` (including SSH connection failures to the machine)
