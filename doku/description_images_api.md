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
- **Write deadline**: the stream carries its own 10-minute write deadline — registry pulls routinely exceed the global write timeout — and stays open until the pull finishes
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
- 500: `INTERNAL_ERROR` (also for a miswired route missing identity), `STREAMING_NOT_SUPPORTED`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/images/prune

Remove all unused images on a machine in one call — tagged or not, the equivalent of `podman image prune -a`. Images in use by containers are untouched.

**Authentication:** Required

**Permissions:** Write level required (bulk deletion, wide blast radius)

**Response (Success - 200):**
```json
[
  {
    "id": "sha256:abc123...",
    "size": 4096,
    "error": "string, only present when podman reported a per-image removal error"
  }
]
```

One entry per removed image with its ID and reclaimed size (bytes). An empty array means nothing was pruned.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED` (read-level users)
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

### GET /api/machines/{id}/images/{imageId}

Get one image's detail on a machine: the list fields combined with the inspect report's config fields.

**Authentication:** Required

**Permission:** `read` (any grantee of the machine)

**Response (Success - 200):**
```json
{
  "id": "sha256:abc123...",
  "repoTags": ["nginx:latest"],
  "repoDigests": ["nginx@sha256:..."],
  "createdAt": "RFC3339 timestamp",
  "size": 102400000,
  "os": "string (optional)",
  "architecture": "string (optional)",
  "user": "string (optional)",
  "env": { "KEY": "value" } (optional),
  "entrypoint": ["string"] (optional),
  "cmd": ["string"] (optional),
  "labels": { "key": "value" } (optional)
}
```

`repoTags` is nullable (an untagged image answers `null`). `env` is parsed from the inspect report's `KEY=VALUE` strings; `user`, `entrypoint`, `cmd`, and `labels` come from the report's `Config`.

**Response (Error):**
- 400: `MISSING_IMAGE_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`, `IMAGE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/machines/{id}/images/{imageId}/inspect

Inspect one image on a machine. The response is the raw podman inspect report as the top-level JSON body — no curated DTO; the field names are podman's own.

**Authentication:** Required

**Permission:** `read` (any grantee of the machine)

**Response (Success - 200):**
The raw podman image inspect report, e.g.:
```json
{
  "Id": "sha256:abc123...",
  "RepoTags": ["nginx:latest"],
  "RepoDigests": ["nginx@sha256:..."],
  "Os": "linux",
  "Architecture": "amd64",
  "Size": 102400000
}
```

**Response (Error):**
- 400: `MISSING_IMAGE_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`, `IMAGE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/machines/{id}/images/{imageId}/export

Download an image as a tar archive (issue 401): the machine's podman
streams the image's docker-archive straight into the response — no
server-side buffering, no size cap (the transfer bound exists to protect
the cross-machine copy's in-process pipe; the browser consumes this
stream directly, and cancellation rides the request context).

**Authentication:** Required

**Access:** Any user with a machine-access row for `{id}` (read level):
a download mutates nothing, exactly like the volume export.

**Response (Success - 200):**
- `Content-Type: application/x-tar`
- `Content-Disposition: attachment; filename="<shortId>.tar"` — the
  image ID with any `sha256:` prefix stripped, truncated to 12 hex
  characters (the ID is the wire's image reference; a tag-based filename
  would need extra resolution and fails for untagged images)
- Body: the image's tar archive (docker-archive format)

The image's existence is resolved before the stream begins, so an
unknown ID answers the standard error envelope below — no partial-file
download.

**Response (Error):**
- 400: `MISSING_IMAGE_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row)
- 404: `MACHINE_NOT_FOUND`, `IMAGE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/machines/{id}/images/{imageId}/copy-preview

Preview an image copy to another machine (issue 375): the one fact the
copy dialog must show before confirming — whether the target machine
already holds the image by ID. Information, never a block: the load is
idempotent (images are content-addressed), so confirming stays possible
either way.

**Authentication:** Required

Read-level access on the source machine suffices (the endpoint mutates
nothing). The caller additionally needs **at least read access on the
target machine**: the target's image store is read server-side, and a
caller without target access gets the standard `ACCESS_DENIED` error —
never a false preview. An unknown target machine ID answers
`ACCESS_DENIED` the same way (no access row can exist for it);
`MACHINE_NOT_FOUND` is reserved for the source machine in the path.

**Query Parameters:**
- `targetMachineId` (string, required): ID of the target machine the
  preview refers to.

**Response (Success - 200):**
```json
{
  "present": boolean
}
```

- `present`: true exactly when the target machine already holds the
  image by ID. The copy is copy-only and idempotent — a present target
  image never refuses anything, the flag only informs the dialog.

**Response (Error):**
- 400: `MISSING_IMAGE_ID`, `MISSING_TARGET_MACHINE_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row on the source, or below-read level
  on the target)
- 404: `MACHINE_NOT_FOUND`, `IMAGE_NOT_FOUND` (the source image vanished)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED` (source or target unreachable)

---

### POST /api/machines/{id}/images/{imageId}/copy

Copy an image to another machine (issue 375): the source machine's
podman streams the image's archive into the target machine's podman as a
live-streamed transfer. **Copy-only** — no move mode, no `force`, no
source removal; the source image is never touched. The full image ID is
the wire's image reference; the archive preserves the image's tags.

**Idempotent by construction, no rollback:** images are content-addressed,
so loading onto a target that already holds the image skips present
layers and re-applies tags — never a collision refusal — and a failed
load leaves already-transferred layers on the target that a retry skips.

**Transfer bound:** the archive stream is bounded by
`IMAGE_TRANSFER_MAX_BYTES` (default 10 GiB — images are routinely larger
than the volume transfer's 2 GiB). Crossing it answers
`IMAGE_TRANSFER_TOO_LARGE`.

**Transport is ALWAYS NDJSON streaming** — no plain/stream fork, one code
path: the response is a stream of newline-delimited events, archive
progress, then exactly one terminal event. The stream carries its own
10-minute write deadline; a client disconnect cancels the operation and
the stream ends with the terminal `OPERATION_CANCELLED` error event.
Errors answered BEFORE the stream begins (bad param, malformed body) keep
the plain error envelope.

**Authentication:** Required

**Write-level access on the source machine is required** (the transfer
mutates remote state through it). The caller additionally needs **at
least write access on the target machine**: route middleware only sees
the source machine, so the target's level is checked server-side against
the access store BEFORE any podman call — a below-write grantee (or a
caller with no target row) gets the standard `ACCESS_DENIED` riding the
stream's terminal error event, with nothing transferred. An unknown
target machine ID answers `ACCESS_DENIED` the same way;
`MACHINE_NOT_FOUND` is reserved for the source machine in the path.

**Request:**
```json
{
  "targetMachineId": "string (required)"
}
```

**Progress events (`type: "image"`):** `phase: "start"` when the archive
stream begins, `phase: "bytes"` as the streamed count crosses each MiB
(carrying `bytes`), `phase: "done"` when the load finished (carrying the
total `bytes`). All events share one `operationId`, the source
`machineId`, the `image` ID, and a `timestamp`.

**Terminal events:** on success one `type: "complete"` event carrying the
transfer envelope in `result`:

```json
{
  "id": "string (the copied image's full ID)",
  "status": "copied",
  "sourceMachineId": "string",
  "targetMachineId": "string",
  "bytes": integer,
  "names": ["string"]
}
```

- `bytes`: the archive that crossed the export→load splice.
- `names`: the image names the target's load reported (the tags the load
  applied). Empty is `[]`, never `null`.

On failure one `type: "error"` event carrying `error: {code, message}`;
the status codes ride this event instead of HTTP statuses (the stream
itself answers 200):

- `IMAGE_NOT_FOUND` (404 semantics: the source image vanished before the
  stream began)
- `IMAGE_TRANSFER_TOO_LARGE` (413 semantics: the archive crossed
  `IMAGE_TRANSFER_MAX_BYTES` — the message names the image and byte
  count)
- `IMAGE_TRANSFER_FAILED` (500 semantics: an engine failure during the
  transfer — the message names the image and cause; already-transferred
  layers stay on the target and a retry skips them)
- plus the transfer's own codes (`ACCESS_DENIED`, `CONNECTION_FAILED`,
  `OPERATION_CANCELLED`, ...)

**Response (Error)** — plain envelope, before the stream begins:
- 400: `MISSING_IMAGE_ID`, `INVALID_REQUEST` (malformed body),
  `VALIDATION_FAILED` (missing `targetMachineId`)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row on the source)
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
