# Podman Management API: Pod Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

Pods live only in the remote host's podman; the backend proxies podman's service API and persists nothing locally. Listing, the per-pod detail endpoint, the raw inspect endpoint, and the pod's aggregated logs are available at `read` level; creating a pod, pruning, the lifecycle actions (start, stop, pause, unpause, kill, restart), and removing mutate remote state and gate at `write` level.

---

## Pod Endpoints

### GET /api/hosts/{id}/pods

List every pod on a host — everything podman reports, including stopped pods. No filtering, no built-in marking; the list reflects the host's real state.

**Authentication:** Required

**Access:** Any user with a host-access row for `{id}` (read level).

**Response (Success - 200):**
```json
{
  "pods": [
    {
      "id": "string",
      "name": "string",
      "state": "string (podman's status: Running, Exited, Created, Paused, Degraded, Stopped)",
      "createdAt": "RFC3339 timestamp",
      "containerCount": "integer (infra container included)",
      "networks": ["string (network names on the infra container)"],
      "labels": { "string": "string" }
    }
  ],
  "total": "integer",
  "timestamp": "RFC3339 timestamp"
}
```

Notes:
- `pods` is always present; it is `[]` when the host has no pods.
- `networks` and `labels` are omitted when empty.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/hosts/{id}/pods

Create a pod on the host. The body carries the pod's `name` (required), the networks its infra container joins, named volumes, bind mounts, and port mappings in the same shapes as the container create request, labels, and resource limits. The infra container stays at podman's default.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Request body:**
```json
{
  "name": "string (required, max 255)",
  "networks": ["string (network name, optional)"],
  "volumes": [
    {
      "name": "string (named volume, required)",
      "destination": "string (path inside the pod, required)",
      "mode": "string (\"ro\" or \"rw\", optional, defaults to \"rw\")"
    }
  ],
  "mounts": [
    {
      "source": "string (host path, required)",
      "destination": "string (path inside the pod, required)",
      "mode": "string (\"ro\" or \"rw\", optional, defaults to \"rw\")",
      "selinuxShared": "boolean (z relabel, optional)",
      "selinuxPrivate": "boolean (Z relabel, optional)",
      "chownUser": "boolean (U, chown to the container's user, optional)"
    }
  ],
  "portMappings": [
    {
      "hostPort": "integer (port on the host, required, 1-65535)",
      "containerPort": "integer (port inside the pod, required, 1-65535)",
      "protocol": "string (\"tcp\" or \"udp\", optional, defaults to \"tcp\")"
    }
  ],
  "labels": { "string": "string" },
  "cpus": "number (CPU limit in cores, optional)",
  "memory": "string (podman units syntax, e.g. \"512m\", \"1g\", optional)"
}
```

Notes:
- `selinuxShared` (z) and `selinuxPrivate` (Z) are mutually exclusive; a mount carrying both is refused with 400 `VALIDATION_FAILED`.
- `portMappings` publish on the pod's infra container; the protocol defaults to `tcp` when omitted. An out-of-range port or a protocol other than `tcp`/`udp` is refused with 400 `VALIDATION_FAILED`. The published mappings appear on the infra container in the raw inspect report (`GET .../pods/{podId}/inspect`).
- `cpus` maps onto the pod spec's CPU limit (quota = cpus × 100000, period 100000).
- `memory` parses through podman's units parser into a byte limit; an unparseable value is 400 `VALIDATION_FAILED`.

**Response (Success - 201):**
```json
{
  "id": "string",
  "name": "string",
  "state": "\"created\"",
  "createdAt": "RFC3339 timestamp"
}
```

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed body; podman-rejected names and other podman-side 4xx rejections, carrying podman's message), `VALIDATION_FAILED` (missing name; a mount with both `selinuxShared` and `selinuxPrivate`; an out-of-range port or unknown protocol in `portMappings`; unparseable `memory`)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/hosts/{id}/pods/prune

Remove all stopped pods on the host in one call (podman's own selection). Running pods are untouched.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):**
```json
[
  { "id": "string", "error": "string (optional, per-pod removal failure)" }
]
```

Notes:
- Empty result is `[]`, never `null`.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/hosts/{id}/pods/remove

Remove several pods by id in one request. Each id is removed through the
same classification as the single-item `DELETE /api/hosts/{id}/pods/{podId}`,
so a pod whose state refuses the removal without `force` is refused in its
own report entry while the rest of the batch still processes.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Request Body:**
```json
{
  "ids": ["pod-id-a", "pod-id-b"],
  "force": false
}
```

- `ids` (array of strings, required): the pod ids to remove. Must be
  non-empty and carry no empty entry.
- `force` (boolean, optional, default false): with `force=true` the pod's
  running containers are removed too, exactly like the single-item
  delete's `force` query flag.

**Response (Success - 200):** one entry per requested id, in request
order.
```json
[
  { "id": "pod-id-a", "success": true },
  {
    "id": "pod-id-b",
    "success": false,
    "error": { "code": "POD_STATE_CONFLICT", "message": "string (podman's refusal text)" }
  }
]
```

Notes:
- A per-id failure is a report entry, never a status code: the batch
  answers 200 even when every entry failed.
- `error` is present only on a failed entry. Its `code` is the same code
  the single-item delete would answer — `POD_NOT_FOUND`,
  `POD_STATE_CONFLICT` (removing a running pod without `force`; the
  message carries podman's refusal text), or `INTERNAL_ERROR` for an
  unclassified cause (whose detail is never surfaced).
- The report is `[]`, never `null`.

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed body), `VALIDATION_FAILED` (missing
  or empty `ids`, or an empty entry)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### GET /api/hosts/{id}/pods/{podId}

Return one pod's detail DTO — the fields the pod detail page renders, mapped from podman's inspect report: name, id, state, creation time, labels, restart policy, mounts as `--mount`-style strings, the pod's containers (id, name, state, ordered by container ID), and the networks on the infra container.

**Authentication:** Required

**Access:** Any user with a host-access row for `{id}` (read level).

**Response (Success - 200):**
```json
{
  "id": "string",
  "name": "string",
  "state": "string (podman's status: Running, Exited, Created, Paused, Degraded, Stopped)",
  "createdAt": "RFC3339 timestamp",
  "labels": { "string": "string" },
  "restartPolicy": "string (podman's effective policy)",
  "mounts": ["string (\"type=…,source=…,destination=…\")"],
  "containers": [
    {
      "id": "string (container ID)",
      "name": "string (container name)",
      "state": "string (container state)"
    }
  ],
  "networks": ["string (network names on the infra container)"]
}
```

Notes:
- `containers` is always present; it is `[]` when the pod has no containers.
- `createdAt` is an RFC3339 timestamp.
- This endpoint is distinct from `GET .../pods/{podId}/inspect`, which returns the raw, unmapped inspect report.

**Response (Error):**
- 400: `MISSING_POD_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `POD_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### GET /api/hosts/{id}/pods/{podId}/inspect

Return one pod's raw podman inspect report — the full pod configuration podman holds (infra config, containers, mounts, networks, labels), marshaled as-is with no DTO mapping. The report is the top-level JSON body.

**Authentication:** Required

**Access:** Any user with a host-access row for `{id}` (read level).

**Response (Success - 200):** the raw pod inspect report as the top-level JSON body — podman's `PodInspectReport` shape, not the detail DTO.

**Response (Error):**
- 400: `MISSING_POD_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `POD_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/hosts/{id}/pods/{podId}/pause

Pause every running container in the pod. Pausing an already-paused pod is refused by podman and surfaces as 409 `POD_STATE_CONFLICT`.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):**
```json
{
  "id": "string (the pod the action ran against)",
  "state": "\"paused\"",
  "timestamp": "RFC3339 timestamp"
}
```

**Response (Error):**
- 400: `MISSING_POD_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `HOST_NOT_FOUND`, `POD_NOT_FOUND`
- 409: `POD_STATE_CONFLICT` (podman refuses the action for the pod's current state; the message carries podman's refusal text)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/hosts/{id}/pods/{podId}/unpause

Unpause every paused container in the pod.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):** the action response shape above, with `"state": "running"`.

**Response (Error):** same as pause.

### POST /api/hosts/{id}/pods/{podId}/kill

Send podman's default signal (SIGKILL) to the pod's running containers. Takes no signal parameter.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):** the action response shape above, with `"state": "exited"`.

**Response (Error):** same as pause.

### POST /api/hosts/{id}/pods/{podId}/restart

Restart the pod's containers.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):** the action response shape above, with `"state": "running"`.

**Response (Error):** same as pause.

### POST /api/hosts/{id}/pods/{podId}/start

Start the pod's containers. Takes no timeout or force parameters — podman's defaults apply. Starting an already-running pod is refused by podman and surfaces as 409 `POD_STATE_CONFLICT`.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):** the action response shape above, with `"state": "running"`.

**Response (Error):** same as pause.

### POST /api/hosts/{id}/pods/{podId}/stop

Stop the pod's containers. Takes no timeout or force parameters — podman's defaults apply. Stopping an already-stopped pod is refused by podman and surfaces as 409 `POD_STATE_CONFLICT`.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):** the action response shape above, with `"state": "stopped"`.

**Response (Error):** same as pause.

### GET /api/hosts/{id}/pods/{podId}/logs

Get the pod's aggregated logs — every container in the pod, infra
included, as plain text with no per-container prefixing.

**Authentication:** Required

**Access:** Any user with a host-access row for `{id}` (read level).

**Query Parameters:**
- `follow` (boolean, optional): Follow log output as an NDJSON stream
- `since` (string, optional): Show logs since timestamp (RFC3339)
- `tail` (integer, optional): Number of lines to show from the end

**Response (Success - 200):**
```json
{
  "id": "string",
  "logs": "string (multiline log content)"
}
```

For streaming logs (`follow=true`):
- **Content-Type**: `application/json`
- **Format**: Newline-delimited JSON (NDJSON)
- Each line is a log line object:

```json
{
  "timestamp": "ISO8601 timestamp",
  "level": "INFO|WARN|ERROR",
  "message": "string"
}
```

The stream stays open while the pod runs and terminates with exactly
one terminal event:

- A clean stream end (pod stopped, podman EOF) emits `{"level": "INFO", "message": "STREAM_ENDED"}`.
- A failure emits an ERROR-level event whose message embeds the error code:
  `"POD_NOT_FOUND: Pod not found"`, `"CONNECTION_FAILED: Failed to connect to host"`, etc.
- A client disconnect cancels the stream server-side and emits a WARN
  `OPERATION_CANCELLED` event (to the already-closed connection).

**Response (Error):**
- 400: `INVALID_SINCE_PARAM` / `INVALID_TAIL_PARAM` (malformed `since`/`tail`, message says "use RFC3339"; answered through the JSON envelope even when `follow=true`), `MISSING_POD_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `POD_NOT_FOUND`
- 500: `INTERNAL_ERROR` (also for a miswired route missing identity), `STREAMING_NOT_SUPPORTED`
- 502: `CONNECTION_FAILED`
- 504: `OPERATION_CANCELLED` (buffered fetch cancelled by a client disconnect; follow-mode cancellation is the WARN terminal event above)

### DELETE /api/hosts/{id}/pods/{podId}

Remove a pod. Running containers refuse the removal unless `force=true` is passed, mirroring the container delete.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Query parameters:**
- `force` (boolean, optional, default `false`): also remove the pod's running containers.

**Response (Success - 204):** no body.

**Response (Error):**
- 400: `MISSING_POD_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `HOST_NOT_FOUND`, `POD_NOT_FOUND`
- 409: `POD_STATE_CONFLICT` (removing a running pod without force; the message carries podman's refusal text)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`
