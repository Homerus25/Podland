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
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### GET /api/machines/{id}/volumes/{volumeName}

Inspect a single volume by exact name, returning its full configuration.

**Authentication:** Required

**Access:** Any user with a machine-access row for `{id}` (read level).

**Response (Success - 200):**
```json
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
```

Notes:
- The volume is resolved by exact name; names are case-sensitive.
- `size` is only present when podman reports usage for the volume.
- `labels` and `options` are always present; they are `{}` when empty.

**Response (Error):**
- 400: `MISSING_VOLUME_NAME` (empty volume name)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND` (unknown machine ID), `VOLUME_NOT_FOUND` (unknown volume name)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/machines/{id}/volumes

Create a volume on the machine. The body accepts the podman create
surface: optional `name` (podman generates one when omitted, yielding an
anonymous volume), `labels`, `driver` (defaults to podman's `local`),
and `options` (driver options). Name-format validation is podman's job.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Request body:**
```json
{
  "name": "string (optional; empty creates an anonymous volume)",
  "labels": { "key": "value" },
  "driver": "string (optional)",
  "options": { "key": "value" }
}
```

**Response (Success - 201):** the created volume's full config (same
shape as inspect, including the generated name for anonymous volumes).

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed body; podman-rejected names/drivers)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `MACHINE_NOT_FOUND`, `VOLUME_NOT_FOUND` (existence re-check)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/machines/{id}/volumes/prune

Remove all unused volumes on the machine in one call. Volumes in use by
containers are untouched.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):**
```json
[
  { "name": "string", "size": integer, "error": "string (optional, per-volume removal failure)" }
]
```

Notes:
- Empty result is `[]`, never `null`.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### GET /api/machines/{id}/volumes/{volumeName}/export

Stream the volume's contents as a tar archive download. Bytes flow
straight from the remote podman into the response — no server-side
buffering, no size cap on the download path.

**Authentication:** Required

**Access:** Any user with a machine-access row for `{id}` (read level):
users who can read a machine's state can read its data.

**Response (Success - 200):**
- `Content-Type: application/x-tar`
- `Content-Disposition: attachment; filename="<volumeName>.tar"`
- Body: the tar stream

**Response (Error):**
- 400: `MISSING_VOLUME_NAME`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row)
- 404: `MACHINE_NOT_FOUND`, `VOLUME_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/machines/{id}/volumes/{volumeName}/populate

Fill a volume from an upload. Body is `multipart/form-data` in exactly
one of two shapes:

- **files**: one or more `files` parts. Each part's filename is the
  relative path inside the volume and is preserved (directory segments
  included).
- **archive**: exactly one `archive` part containing a ready-made tar,
  imported as-is.

Sending both shapes, or neither, is a 400.

**Semantics are podman-native MERGE: uploaded paths overwrite
same-named files; everything else in the volume is kept. There is no
replace mode.**

Uploads are bounded by `VOLUME_UPLOAD_MAX_BYTES` (default 2 GiB);
parts are streamed to temp files (never buffered in RAM) and the cap is
enforced while streaming. Temp files are removed on every path.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 204):** no body.

**Response (Error):**
- 400: `INVALID_REQUEST` (non-multipart body, both/neither shapes, path
  traversal in a filename, missing filename)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `MACHINE_NOT_FOUND`, `VOLUME_NOT_FOUND`
- 413: `VOLUME_UPLOAD_TOO_LARGE` (upload exceeded `VOLUME_UPLOAD_MAX_BYTES`)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### DELETE /api/machines/{id}/volumes/{volumeName}

Remove a volume by exact name. Query flag `force` (default false): with
`force=true` podman stops dependent containers first and removes the
volume anyway. A non-boolean `force` value is ignored (treated as
false), matching every other boolean query flag in the API.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Query Parameters:**
- `force` (boolean, optional, default false; non-boolean values are
  ignored and treated as false)

**Response (Success - 204):** no body.

**Response (Error):**
- 400: `MISSING_VOLUME_NAME`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `MACHINE_NOT_FOUND`, `VOLUME_NOT_FOUND`
- 409: `VOLUME_IN_USE` (volume is in use by a container and `force` was
  not set; carries podman's message)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`
