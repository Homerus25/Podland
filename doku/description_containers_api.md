# Podman Management API: Container Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

---

## Container Endpoints

### GET /api/machines/{id}/containers

List all containers on a machine.

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
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/containers

Create a new container on a machine.

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
- 404: `MACHINE_NOT_FOUND`, `IMAGE_NOT_FOUND`, `POD_NOT_FOUND` (a create that requested a pod which does not exist)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/machines/{id}/containers/{containerId}

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
  "config": { ... }
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
sorted by name.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/machines/{id}/containers/{containerId}/inspect

Get the raw podman inspect report for a specific container. The response
body is podman's inspect report verbatim as the top-level JSON object —
no curated DTO mapping (unlike `GET .../containers/{containerId}`).

**Authentication:** Required

**Permission:** `read` (any grantee of the machine)

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
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/containers/{containerId}/start

Start a stopped container.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string",
  "state": "running",
  "startedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `CONTAINER_ALREADY_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/containers/{containerId}/stop

Stop a running container.

**Authentication:** Required

**Request:**
```json
{
  "force": boolean (optional, default: false),
  "timeout": integer (optional, seconds, default: 10)
}
```

**Response (Success - 200):**
```json
{
  "id": "string",
  "state": "stopped",
  "finishedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `CONTAINER_NOT_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/containers/{containerId}/restart

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
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### DELETE /api/machines/{id}/containers/{containerId}

Remove a container.

**Authentication:** Required

**Query Parameters:**
- `force` (boolean, optional): Force removal of running container
- `volumes` (boolean, optional): Remove associated volumes

**Response (Success - 204):**
- No body

**Response (Error):**
- 400: `CONTAINER retroced` (if running and force not set)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/machines/{id}/containers/{containerId}/logs

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
  `"CONTAINER_NOT_FOUND: Container not found"`, `"CONNECTION_FAILED: Failed to connect to machine"`, etc.
- A client disconnect cancels the stream server-side and emits a WARN
  `OPERATION_CANCELLED` event (to the already-closed connection).

**Response (Error):**
- 400: `INVALID_SINCE_PARAM` / `INVALID_TAIL_PARAM` (malformed `since`/`tail`, message says "use RFC3339"; answered through the JSON envelope even when `follow=true`), `MISSING_CONTAINER_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR` (also for a miswired route missing identity), `STREAMING_NOT_SUPPORTED`
- 502: `CONNECTION_FAILED`
- 504: `OPERATION_CANCELLED` (buffered fetch cancelled by a client disconnect; follow-mode cancellation is the WARN terminal event above)
