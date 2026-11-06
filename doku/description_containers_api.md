# Podman Management API: Container Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

---

## Container Endpoints

### GET /api/hosts/{id}/containers

List all containers on a host.

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
    "imageId": "string",
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
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/containers

Create a new container on a host.

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
      "hostPort": integer (required, 1-65535),
      "containerPort": integer (required, 1-65535),
      "protocol": "tcp|udp" (optional, default: tcp)
    }
  ] (optional),
  "mounts": [
    {
      "source": "string (required)",
      "destination": "string (required)",
      "mode": "ro|rw" (optional, default: rw),
      "selinuxShared": boolean (optional, default: false, SELinux shared relabel 'z'),
      "selinuxPrivate": boolean (optional, default: false, SELinux private relabel 'Z'),
      "chownUser": boolean (optional, default: false, chown the mounted content to the container's user 'U')
    }
  ] (optional),
  "volumes": [
    {
      "name": "string (required)",
      "destination": "string (required)",
      "mode": "ro|rw" (optional, default: rw)
    }
  ] (optional),
  "networks": ["string"] (optional),
  "pod": "string (optional, name or ID of an existing pod the new container joins)",
  "secrets": [
    {
      "source": "string (required)",
      "target": "string (optional, default: /run/secrets/<source>)"
    }
  ] (optional),
  "devices": [
    {
      "hostPath": "string (required)",
      "containerPath": "string (required)"
    }
  ] (optional),
  "privileged": boolean (optional, default: false, run the container in privileged mode),
  "capAdd": ["string"] (optional, Linux capability names e.g. 'NET_ADMIN', passed to podman verbatim — an unknown name is refused by podman),
  "restartPolicy": "no|always|unless-stopped|on-failure" (optional, default: no),
  "remove": boolean (optional, default: false, remove the container when it exits),
  "user": "string (optional, UID or username)",
  "timezone": "string (optional, IANA name e.g. 'Europe/Berlin')",
  "memory": "string (podman units syntax, e.g. \"512m\", \"1g\", optional)",
  "cpus": "number (CPU limit in cores, optional)"
}
```

The `portBindings`, `mounts`, `volumes`, `secrets`, and `devices` arrays
validate each entry's required sub-fields (a missing `hostPort`,
`source`, `name`, `destination`, or device path is a 400
`VALIDATION_FAILED`). `mounts` are applied as bind mounts with `rbind`
propagation; a mount's `selinuxShared`, `selinuxPrivate`, and `chownUser`
flags ride along as the `z`, `Z`, and `U` mount options, and a mount
carrying both `selinuxShared` and `selinuxPrivate` is refused with 400
`VALIDATION_FAILED` (the labels are mutually exclusive);
`volumes` are applied as named volumes; `networks` attaches
the container to each named network; a `device` whose `containerPath`
differs from its `hostPath` is additionally bind-mounted at the
container path. `privileged` runs the container in privileged mode;
`capAdd` grants the listed Linux capabilities, with each name passed to
podman verbatim (an unknown capability name is refused by podman and
surfaces as an error, not a silent ignore). `cpus` maps onto the
container spec's CPU limit (quota = cpus × 100000, period 100000);
`memory` parses through podman's units parser into a byte limit, and an
unparseable `memory` value is refused with 400 `VALIDATION_FAILED` (a
message naming the field and the expected syntax, e.g. "512m", "1g").
`pod` names (or IDs) an existing pod the
new container joins; when set, the value is passed to podman verbatim
and the pod owns the container's networking (a `networks` list alongside
a `pod` is podman's refusal to answer). A create whose requested pod
does not exist answers 404 `POD_NOT_FOUND`; without a `pod` requested, a
not-found create stays `IMAGE_NOT_FOUND`. The classification is
ambiguous by nature — podman's not-found answer covers the image, the
pod, and anything else in the spec — so the envelope's `message` carries
podman's own not-found text as the disambiguator.

**Response (Success - 201):**
```json
{
  "id": "string",
  "name": "string",
  "state": "created|running",
  "image": "string",
  "imageId": "string",
  "createdAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `VALIDATION_FAILED` (missing image; a mount with both `selinuxShared` and `selinuxPrivate`; unparseable `memory`), `INVALID_REQUEST`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `IMAGE_NOT_FOUND`, `POD_NOT_FOUND` (a create that requested a pod which does not exist)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/containers/prune

Remove all stopped containers on the host in one call. Running
containers are untouched.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):**
```json
[
  { "id": "string", "size": integer, "error": "string (optional, per-container removal failure)" }
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

---

### POST /api/hosts/{id}/containers/remove

Remove several containers by id in one request. Each id is removed
through the same classification as the single-item
`DELETE /api/hosts/{id}/containers/{containerId}`, so a running (or
paused) container without `force` is refused in its own report entry
while the rest of the batch still processes.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Request Body:**
```json
{
  "ids": ["container-id-a", "container-id-b"],
  "force": false
}
```

- `ids` (array of strings, required): the container ids to remove. Must
  be non-empty and carry no empty entry.
- `force` (boolean, optional, default false): with `force=true` podman
  removes the container regardless of its state — stopping a running one
  first.

**Response (Success - 200):** one entry per requested id, in request
order.
```json
[
  { "id": "container-id-a", "success": true },
  {
    "id": "container-id-b",
    "success": false,
    "error": { "code": "CONTAINER_RUNNING", "message": "string (podman's refusal text)" }
  }
]
```

Notes:
- A per-id failure is a report entry, never a status code: the batch
  answers 200 even when every entry failed.
- `error` is present only on a failed entry. Its `code` is the same code
  the single-item delete would answer — `CONTAINER_NOT_FOUND`,
  `CONTAINER_RUNNING`, or `INTERNAL_ERROR` for an unclassified cause
  (whose detail is never surfaced).
- The report is `[]`, never `null`.

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed body), `VALIDATION_FAILED` (missing
  or empty `ids`, or an empty entry)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED` (read-level users)
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/hosts/{id}/containers/stats

One-shot resource-usage snapshot for every running container on the
host. Stats are sampled once per call (no streaming); stopped or
created containers do not appear.

**Authentication:** Required

**Permission:** `read` (any grantee of the host)

**Response (Success - 200):**
```json
{
  "containers": [
    {
      "id": "string",
      "name": "string",
      "cpuPercent": 0.0,
      "memoryUsage": 0,
      "memoryLimit": 0,
      "memoryPercent": 0.0,
      "networkBytesIn": 0,
      "networkBytesOut": 0,
      "blockBytesIn": 0,
      "blockBytesOut": 0,
      "pids": 0
    }
  ],
  "total": 0,
  "timestamp": "ISO8601 timestamp"
}
```

Notes:
- All values are raw numbers — no formatting server-side. The network
  byte counters are summed over the container's interfaces.
- Nothing running is a legal 200 with an empty `containers` array.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/hosts/{id}/containers/{containerId}

Get details of a specific container.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string",
  "name": "string",
  "state": "created|running|paused|stopped|removing|exited|dead",
  "image": "string",
  "imageId": "string",
  "command": "string[]",
  "entrypoint": ["string"],
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
  "mounts": ["string"],
  "secrets": [
    {
      "name": "string",
      "uid": integer,
      "gid": integer,
      "mode": integer
    }
  ],
  "volumes": [
    {
      "name": "string",
      "source": "string",
      "destination": "string",
      "rw": boolean
    }
  ],
  "exposedPorts": [
    {
      "containerPort": integer,
      "protocol": "tcp|udp"
    }
  ],
  "environment": { "key": "value" },
  "labels": { "key": "value" },
  "networks": ["string"],
  "logPath": "string",
  "config": { ... },
  "limits": {
    "cpus": "number (from the inspect report's CPU quota/period; 0 when unset)",
    "memory": "string (podman units syntax e.g. \"512m\"; empty when unset)"
  },
  "healthcheck": {
    "command": "string (from the inspect report's test array; empty when disabled)",
    "interval": "string (Go duration e.g. \"30s\"; omitted when inherited)",
    "timeout": "string (Go duration; omitted when inherited)",
    "startPeriod": "string (Go duration; omitted when inherited)",
    "retries": "integer",
    "disabled": "boolean (true when the test array is NONE)"
  }
}
```

The `entrypoint`, `secrets`, `volumes`, `exposedPorts`, `mounts`, and
`networks` fields are optional (omitted when empty). `entrypoint` comes
from the inspect report's `Config.Entrypoint`; `secrets` from
`Config.Secrets` (the secrets mounted in the container); `volumes`
lists the named volume mounts filtered from the inspect report's
`Mounts` (only entries of type `volume`); `exposedPorts` comes from
`Config.ExposedPorts`, sorted by container port; `mounts` lists every
mount from the inspect report's `Mounts` as `--mount`-style strings
(`type=<type>,source=<source>,destination=<destination>` — named volumes
and binds alike, in report order; the mount's `rw` flag is not included in
these display strings — use the structured `volumes` entries for read-write
state); `networks` lists the names of the networks the container is
attached to (the keys of the inspect report's `NetworkSettings.Networks`),
sorted by name. The `limits` and `healthcheck` fields are optional too
(omitted when the container carries no limits / no healthcheck): `limits`
reports the current resource limits — `cpus` derived from the inspect
report's CPU quota/period, `memory` rendered back into podman units
syntax — and `healthcheck` reports the current healthcheck — the command
derived from the inspect report's test array (`CMD` arguments or a
`CMD-SHELL` command joined into one string; a `NONE` test reports
`disabled: true`), durations as Go duration strings (a zero duration
means "inherit podman's default" and is omitted). Together they prefill
the edit-configuration form served by `POST .../containers/{containerId}/update`.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/hosts/{id}/containers/{containerId}/inspect

Get the raw podman inspect report for a specific container. The response
body is podman's inspect report verbatim as the top-level JSON object —
no curated DTO mapping (unlike `GET .../containers/{containerId}`).

**Authentication:** Required

**Permission:** `read` (any grantee of the host)

**Response (Success - 200):**
The raw podman container inspect report as the top-level JSON body:
```json
{
  "Id": "string",
  "Name": "string",
  "Created": "ISO8601 timestamp",
  "Image": "string",
  "State": { "...": "podman's InspectContainerState, verbatim" },
  "Config": { "...": "podman's inspect config, verbatim" }
}
```
The body is podman's full `ContainerInspectReport` — every field podman
returns, unmapped.

**Response (Error):**
- 400: `MISSING_CONTAINER_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/containers/{containerId}/start

Start a stopped container. A container that is already running
(including a paused one) answers `CONTAINER_ALREADY_RUNNING`; use the
restart endpoint for a running container.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string",
  "name": "string",
  "state": "running",
  "timestamp": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `CONTAINER_ALREADY_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/containers/{containerId}/stop

Stop a running container. The endpoint accepts no request body — no
timeout or force parameters travel (podman's default timeout applies).
A container that is not running answers `CONTAINER_NOT_RUNNING`.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string",
  "name": "string",
  "state": "stopped",
  "timestamp": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `CONTAINER_NOT_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/containers/{containerId}/restart

Restart a running container. The endpoint accepts no timeout or force
parameters — podman's default timeout applies. A container that is not
running answers the same `CONTAINER_NOT_RUNNING` terminal as stop; use
the start endpoint for a stopped container.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string",
  "name": "string",
  "state": "running",
  "timestamp": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `CONTAINER_NOT_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/containers/{containerId}/rename

Rename a container. Podman renames running and stopped containers alike,
so there is no state restriction and no quadlet-owned restriction. The
new name's shape is validated server-side by podman — the client guards
only the empty and unchanged cases and does not duplicate podman's name
regex.

**Authentication:** Required

**Request:**
```json
{
  "name": "string (required, 1-255 characters)"
}
```

**Response (Success - 200):** the updated container detail DTO, the same
shape as `GET /api/hosts/{id}/containers/{containerId}`, carrying the
new name.

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed body, or a name podman rejects — the
  message carries podman's reason), `VALIDATION_FAILED` (empty or
  over-long name), `MISSING_CONTAINER_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 409: `CONTAINER_ALREADY_EXISTS` (another container on the host
  already carries the new name)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/containers/{containerId}/update

Change an existing container's resource limits (cpus, memory) and
healthcheck configuration. The contract is patch semantics: omitted
fields mean "unchanged" — only the fields the request carries are
forwarded to podman's container update, and podman merges resource
fields, so sending only a memory limit leaves the CPU limit alone, and
vice versa. Clearing a limit is not supported (an empty field means
"unchanged"; podman has no clear semantics). Podman applies limit
changes live to a running container and rewrites a stopped container's
config for the next start, so there is no state restriction. The
healthcheck command accepts podman CLI syntax — a bare command, or a
`CMD`/`CMD-SHELL`-prefixed one. The disable flag is mutually exclusive
with every other healthcheck field (podman refuses the combination, so
the backend answers 400 without a round trip).

**Authentication:** Required

**Request:**
```json
{
  "cpus": "number (optional, positive; omitted or zero means unchanged)",
  "memory": "string (optional, podman units syntax e.g. \"512m\"; omitted or empty means unchanged)",
  "healthcheck": {
    "cmd": "string (optional, podman CLI syntax — bare, CMD-prefixed, or CMD-SHELL-prefixed)",
    "interval": "string (optional, Go duration e.g. \"30s\")",
    "timeout": "string (optional, Go duration e.g. \"5s\")",
    "startPeriod": "string (optional, Go duration e.g. \"0m30s\")",
    "retries": "integer (optional, non-negative)",
    "disabled": "boolean (optional; mutually exclusive with every other healthcheck field)"
  }
}
```

**Response (Success - 200):** the updated container detail DTO, the same
shape as `GET /api/hosts/{id}/containers/{containerId}`, carrying the
new limits and healthcheck.

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed body), `VALIDATION_FAILED` (an
  unparseable memory value, a malformed duration, a non-positive cpus
  value, or the disable flag combined with another healthcheck field —
  the message names the field), `MISSING_CONTAINER_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### DELETE /api/hosts/{id}/containers/{containerId}
Remove a container by ID. Query flag `force` (default false): with
`force=true` podman removes the container regardless of its state —
stopping a running one first. Without `force` a running (or paused)
container makes the removal a 409 `CONTAINER_RUNNING`. A non-boolean
`force` value is ignored (treated as false), matching every other boolean
query flag in the API.

A remove failing at the call itself — the race window where the container
vanished or started between the request and podman's execution, no
per-container report involved — is classified the same way: not-found
answers 404 `CONTAINER_NOT_FOUND`, a running refusal 409
`CONTAINER_RUNNING` carrying podman's message.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Query Parameters:**
- `force` (boolean, optional, default false; non-boolean values are
  ignored and treated as false)

**Response (Success - 204):**
- No body

**Response (Error):**
- 400: `MISSING_CONTAINER_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 409: `CONTAINER_RUNNING` (the container is running or paused and `force`
  was not set; carries podman's message)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/hosts/{id}/containers/{containerId}/logs

Get logs for a container.

**Authentication:** Required

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

The stream stays open while the container runs and terminates with exactly
one terminal event:

- A clean stream end (container stopped, podman EOF) emits `{"level": "INFO", "message": "STREAM_ENDED"}`.
- A failure emits an ERROR-level event whose message embeds the error code:
  `"CONTAINER_NOT_FOUND: Container not found"`, `"CONNECTION_FAILED: Failed to connect to host"`, etc.
- A client disconnect cancels the stream server-side and emits a WARN
  `OPERATION_CANCELLED` event (to the already-closed connection).

**Response (Error):**
- 400: `INVALID_SINCE_PARAM` / `INVALID_TAIL_PARAM` (malformed `since`/`tail`, message says "use RFC3339"; answered through the JSON envelope even when `follow=true`), `MISSING_CONTAINER_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR` (also for a miswired route missing identity), `STREAMING_NOT_SUPPORTED`
- 502: `CONNECTION_FAILED`
- 504: `OPERATION_CANCELLED` (buffered fetch cancelled by a client disconnect; follow-mode cancellation is the WARN terminal event above)

---

### GET /api/hosts/{id}/containers/{containerId}/stats

One-shot resource-usage snapshot for one container. Stats are sampled
once per call (no streaming); the container must be running.

**Authentication:** Required

**Permission:** `read` (any grantee of the host)

**Response (Success - 200):**
```json
{
  "id": "string",
  "name": "string",
  "cpuPercent": 0.0,
  "memoryUsage": 0,
  "memoryLimit": 0,
  "memoryPercent": 0.0,
  "networkBytesIn": 0,
  "networkBytesOut": 0,
  "blockBytesIn": 0,
  "blockBytesOut": 0,
  "pids": 0
}
```

Notes:
- All values are raw numbers — no formatting server-side. The network
  byte counters are summed over the container's interfaces.

**Response (Error):**
- 400: `MISSING_CONTAINER_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND` (unknown container and
  known-but-not-running container alike — podman's stats handler maps any
  first-report error to 404, so the two cannot be distinguished; podman's
  own message rides along as the error message)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`
