# Podman Management API: Pod Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

Pods live only in the remote machine's podman; the backend proxies podman's service API and persists nothing locally. Listing, the per-pod detail endpoint, and the raw inspect endpoint are available at `read` level; creating a pod, pruning, the lifecycle actions (pause, unpause, kill, restart), and removing mutate remote state and gate at `write` level.

---

## Pod Endpoints

### GET /api/machines/{id}/pods

List every pod on a machine — everything podman reports, including stopped pods. No filtering, no built-in marking; the list reflects the machine's real state.

**Authentication:** Required

**Access:** Any user with a machine-access row for `{id}` (read level).

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
- `pods` is always present; it is `[]` when the machine has no pods.
- `networks` and `labels` are omitted when empty.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/machines/{id}/pods

Create a pod on the machine. The body carries the pod's `name` (required), the networks its infra container joins, named volumes and bind mounts in the same shapes as the container create request, labels, and resource limits. The infra container stays at podman's default.

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
  "labels": { "string": "string" },
  "cpus": "number (CPU limit in cores, optional)",
  "memory": "string (podman units syntax, e.g. \"512m\", \"1g\", optional)"
}
```

Notes:
- `selinuxShared` (z) and `selinuxPrivate` (Z) are mutually exclusive; a mount carrying both is refused with 400 `VALIDATION_FAILED`.
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
- 400: `INVALID_REQUEST` (malformed body; podman-rejected names and other podman-side 4xx rejections, carrying podman's message), `VALIDATION_FAILED` (missing name; a mount with both `selinuxShared` and `selinuxPrivate`; unparseable `memory`)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/machines/{id}/pods/prune

Remove all stopped pods on the machine in one call (podman's own selection). Running pods are untouched.

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
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### GET /api/machines/{id}/pods/{podId}

Return one pod's detail DTO — the fields the pod detail page renders, mapped from podman's inspect report: name, id, state, creation time, labels, restart policy, mounts as `--mount`-style strings, the pod's containers (id, name, state, ordered by container ID), and the networks on the infra container.

**Authentication:** Required

**Access:** Any user with a machine-access row for `{id}` (read level).

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
- 404: `MACHINE_NOT_FOUND`, `POD_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### GET /api/machines/{id}/pods/{podId}/inspect

Return one pod's raw podman inspect report — the full pod configuration podman holds (infra config, containers, mounts, networks, labels), marshaled as-is with no DTO mapping. The report is the top-level JSON body.

**Authentication:** Required

**Access:** Any user with a machine-access row for `{id}` (read level).

**Response (Success - 200):** the raw pod inspect report as the top-level JSON body — podman's `PodInspectReport` shape, not the detail DTO.

**Response (Error):**
- 400: `MISSING_POD_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`, `POD_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/machines/{id}/pods/{podId}/pause

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
- 404: `MACHINE_NOT_FOUND`, `POD_NOT_FOUND`
- 409: `POD_STATE_CONFLICT` (podman refuses the action for the pod's current state; the message carries podman's refusal text)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/machines/{id}/pods/{podId}/unpause

Unpause every paused container in the pod.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):** the action response shape above, with `"state": "running"`.

**Response (Error):** same as pause.

### POST /api/machines/{id}/pods/{podId}/kill

Send podman's default signal (SIGKILL) to the pod's running containers. Takes no signal parameter.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):** the action response shape above, with `"state": "exited"`.

**Response (Error):** same as pause.

### POST /api/machines/{id}/pods/{podId}/restart

Restart the pod's containers.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):** the action response shape above, with `"state": "running"`.

**Response (Error):** same as pause.

### DELETE /api/machines/{id}/pods/{podId}

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
- 404: `MACHINE_NOT_FOUND`, `POD_NOT_FOUND`
- 409: `POD_STATE_CONFLICT` (removing a running pod without force; the message carries podman's refusal text)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`
