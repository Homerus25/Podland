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

A remove failing at the call itself — the race window where the volume
vanished or came into use between the request and podman's execution, no
per-volume report involved — is classified the same way: not-found
answers 404 `VOLUME_NOT_FOUND`, an in-use refusal 409 `VOLUME_IN_USE`
carrying podman's message.

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

### GET /api/machines/{id}/volumes/{volumeName}/move-preview

Preview a volume copy/move to another machine (issue 238): the facts the
transfer dialog must show before confirming — whether the target machine
already holds a same-named volume, whether a container on the source
machine uses the volume, and which source quadlets reference it.

**Authentication:** Required

Read-level access on the source machine suffices (the endpoint mutates
nothing). The caller additionally needs **at least read access on the
target machine**: the target's volume store is read server-side, and a
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
  "collision": boolean,
  "inUse": boolean,
  "referencingQuadlets": [ { "name": "string" } ]
}
```

- `collision`: true exactly when the target machine already holds a
  volume of the same name. The transfer never overwrites or merges — the
  flag drives the dialog's confirm gate, and the transfer re-checks at
  execution time.
- `inUse`: true when any container on the source machine mounts the
  volume (derived from every container's user-declared mounts, the source
  token before the first `:` compared exactly). A move refuses an in-use
  source without `force`; a copy is unaffected.
- `referencingQuadlets`: the source machine's quadlet files whose
  `Volume=` lines resolve to the volume — a bare name matches directly, a
  `.volume` companion source resolves through its companion file's
  `VolumeName=` (podman's `systemd-<name>` default otherwise), host paths
  never match. A warning, never a block; quadlets are not transferred by
  a volume copy/move. Empty is `[]`, never `null`.

**Response (Error):**
- 400: `MISSING_VOLUME_NAME`, `MISSING_TARGET_MACHINE_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row on the source, or below-read level
  on the target)
- 404: `MACHINE_NOT_FOUND`, `VOLUME_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED` (source or target unreachable)

### POST /api/machines/{id}/volumes/{volumeName}/move

Transfer a volume to another machine (issue 238). Two modes:

- **copy** creates the target volume (same name, no rename) and streams
  the source's data into it — always, there is no metadata-only option —
  and never touches the source.
- **move** does the same, then removes the source VOLUME — only after
  the target holds the data (install-first-then-remove). The removal
  honors the delete flow's flags: an in-use source refuses the move with
  `VOLUME_IN_USE` unless `force` is set, and `force` stops dependent
  containers before removal exactly like the delete endpoint.

**Transport is ALWAYS NDJSON streaming** — no plain/stream fork, one code
path: the response is a stream of newline-delimited events, per-volume
progress, then exactly one terminal event. The stream carries its own
10-minute write deadline; a client disconnect cancels the operation and
the stream ends with the terminal `OPERATION_CANCELLED` error event.
Errors answered BEFORE the stream begins (bad param, malformed body) keep
the plain error envelope.

**The collision is re-checked at execution time**: a target volume that
appeared since the preview answers `VOLUME_ALREADY_EXISTS` — never an
overwrite, never a merge. Any failure AFTER the target volume's creation
rolls the half-populated volume back (it must not survive to block a
retry with a false collision); the terminal error event's message says
the rollback ran.

**Authentication:** Required

**Write-level access on the source machine is required** (the transfer
mutates remote state through it). The caller additionally needs **at
least write access on the target machine**: route middleware only sees the
source machine, so the target's level is checked server-side against the
access store BEFORE any podman call — a below-write grantee (or a caller
with no target row) gets the standard `ACCESS_DENIED` riding the stream's
terminal error event, with nothing transferred. An unknown target machine
ID answers `ACCESS_DENIED` the same way; `MACHINE_NOT_FOUND` is reserved
for the source machine in the path.

**Move mode semantics:**

- An in-use source refuses the move with `VOLUME_IN_USE` — before
  anything is created on the target — unless `force` is set.
- A removal failure after the data crossed answers the terminal
  `complete` event with `status: "partial"` and the failure's detail in
  `error`: the volume then exists on BOTH machines and the source side
  must be cleaned up manually. There is no rollback of the target's data.
- A source volume that vanished between the data phase and the removal
  needs no removal — the answer is `status: "moved"`.

The source removal classifies its own failure exactly like the delete
flow: a not-found at the call (the race window where the volume vanished
between the data phase and the removal) is the vanished-source success
(`status: "moved"`), and an in-use refusal answers `status: "partial"`
with the 409 `VOLUME_IN_USE` detail.

**Request:**
```json
{
  "targetMachineId": "string (required)",
  "mode": "copy|move (required)",
  "force": "boolean (optional, move mode only, default false)"
}
```

**Progress events (`type: "volume"`):** `phase: "start"` when the data
stream begins, `phase: "bytes"` as the streamed count crosses each MiB
(carrying `bytes`), `phase: "done"` when the import finished (carrying
the total `bytes`). All events share one `operationId`, the source
`machineId`, the `volume` name, and a `timestamp`.

**Terminal events:** on success one `type: "complete"` event carrying the
transfer envelope in `result`:

```json
{
  "name": "string",
  "status": "copied|moved|partial",
  "sourceMachineId": "string",
  "targetMachineId": "string",
  "bytes": integer,
  "error": "string (optional, present only when status is \"partial\")"
}
```

- `bytes`: the data that crossed the export→import splice.
- `error`: the source removal failure's detail; present only with
  `"partial"`. Clients must narrate the both-machines state from it.

On failure one `type: "error"` event carrying `error: {code, message}`;
the status codes ride this event instead of HTTP statuses (the stream
itself answers 200):

- `VOLUME_ALREADY_EXISTS` (409 semantics: a same-named target volume —
  the message names the volume)
- `VOLUME_IN_USE` (409 semantics: move mode, in-use source, no `force`)
- `VOLUME_UPLOAD_TOO_LARGE` (413 semantics: the streamed data crossed
  `VOLUME_UPLOAD_MAX_BYTES` — the message names the volume and byte
  count, and says the partially transferred target volume was removed)
- `VOLUME_MIGRATION_FAILED` (500 semantics: an engine failure during the
  data phase — the message names the volume and says whether the
  partially transferred target volume was removed)
- plus the transfer's own codes (`ACCESS_DENIED`, `CONNECTION_FAILED`,
  `OPERATION_CANCELLED`, ...)

**Response (Error)** — plain envelope, before the stream begins:
- 400: `MISSING_VOLUME_NAME`, `INVALID_REQUEST` (malformed body),
  `VALIDATION_FAILED` (missing `targetMachineId`, or a `mode` other than
  `"copy"` / `"move"`)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row on the source)
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
