# Podman Management API: Network Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

Networks live only in the remote host's podman; the backend proxies podman's service API and persists nothing locally. Listing and the per-network detail endpoint are available at `read` level; creating a network, connecting a container to one, deleting one, and pruning unused networks mutate remote state and gate at `write` level. Disconnect remains a follow-up issue.

---

## Network Endpoints

### GET /api/hosts/{id}/networks

List every network on a host — everything podman reports, including the default bridge network. No filtering, no built-in marking; the list reflects the host's real state.

**Authentication:** Required

**Access:** Any user with a host-access row for `{id}` (read level).

**Response (Success - 200):**
```json
[
  {
    "name": "string",
    "driver": "string (e.g. \"bridge\", \"macvlan\", \"host\")",
    "subnets": ["string (CIDR, e.g. \"10.88.0.0/16\")"],
    "createdAt": "RFC3339 timestamp"
  }
]
```

Notes:
- The DTO carries exactly these four fields — no id, labels, options, or flags.
- `subnets` is always present; it is `[]` when the network has no subnets.
- `createdAt` is an RFC3339 timestamp.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/hosts/{id}/networks

Create a network on the host. The body carries the network's `name` (required); podman's defaults fill the driver (bridge) and subnet. An optional `containers` list (container IDs) is attached to the new network in the same call: each attach runs after the create, and an attach failure fails the request naming the failing container — the network stays created (no rollback).

**Authentication:** Required

**Access:** Write level on `{id}`.

**Request body:**
```json
{
  "name": "string (required)",
  "containers": ["string (container ID, optional)"]
}
```

**Response (Success - 201):** the created network, in the same shape as the list DTO.
```json
{
  "name": "string",
  "driver": "string (e.g. \"bridge\", \"macvlan\", \"host\")",
  "subnets": ["string (CIDR, e.g. \"10.88.0.0/16\")"],
  "createdAt": "RFC3339 timestamp"
}
```

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed body; podman-rejected names), `VALIDATION_FAILED` (missing name)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND` (attach listed an unknown container)
- 409: `NETWORK_ALREADY_EXISTS` (duplicate name), `NETWORK_ATTACH_FAILED` (attach failure; the message names the failing container and the network stays created)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/hosts/{id}/networks/prune

Remove all unused networks on the host in one call (podman's own selection). Networks in use by a container are untouched.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Response (Success - 200):**
```json
[
  { "name": "string", "error": "string (optional, per-network removal failure)" }
]
```

Notes:
- One entry per removed network: its name and any per-network error podman reported. Podman's network prune report carries no size, so there is no size field.
- Empty result is `[]`, never `null`.

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/hosts/{id}/networks/remove

Remove several networks by exact name in one request. Each name is
removed through the same classification as the single-item
`DELETE /api/hosts/{id}/networks/{networkName}`, so an in-use network
without `force` is refused in its own report entry while the rest of the
batch still processes.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Request Body:**
```json
{
  "ids": ["network-name-a", "network-name-b"],
  "force": false
}
```

- `ids` (array of strings, required): the network names to remove. Must
  be non-empty and carry no empty entry.
- `force` (boolean, optional, default false): with `force=true` podman
  removes the containers attached to the network — stopping any that are
  running — and then removes the network.

**Response (Success - 200):** one entry per requested name, in request
order.
```json
[
  { "id": "network-name-a", "success": true },
  {
    "id": "network-name-b",
    "success": false,
    "error": { "code": "NETWORK_IN_USE", "message": "string (podman's refusal text)" }
  }
]
```

Notes:
- A per-name failure is a report entry, never a status code: the batch
  answers 200 even when every entry failed.
- `error` is present only on a failed entry. Its `code` is the same code
  the single-item delete would answer — `NETWORK_NOT_FOUND`,
  `NETWORK_IN_USE`, or `INTERNAL_ERROR` for an unclassified cause (whose
  detail is never surfaced).
- The report is `[]`, never `null`.

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed body), `VALIDATION_FAILED` (missing
  or empty `ids`, or an empty entry)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### POST /api/hosts/{id}/networks/{networkName}/connect

Attach a container to a network on the host. The body carries the container's ID.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Request body:**
```json
{
  "container": "string (container ID, required)"
}
```

**Response (Success - 200):** an empty JSON object (`{}`).

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed body; podman-rejected attach), `VALIDATION_FAILED` (missing container), `MISSING_NETWORK_NAME`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `HOST_NOT_FOUND`, `NETWORK_NOT_FOUND`, `CONTAINER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### GET /api/hosts/{id}/networks/{networkName}

Return one network's detail DTO — the fields the network detail page renders, mapped from podman's inspect report: name, id, driver, creation time, subnets in CIDR form, and the attached containers (id + name, resolved from the inspect report's `containers` map, ordered by container ID).

**Authentication:** Required

**Access:** Any user with a host-access row for `{id}` (read level).

**Response (Success - 200):**
```json
{
  "name": "string",
  "id": "string (network ID from the inspect report)",
  "driver": "string (e.g. \"bridge\", \"macvlan\", \"host\")",
  "createdAt": "RFC3339 timestamp",
  "subnets": ["string (CIDR, e.g. \"10.89.0.0/16\")"],
  "containers": [
    {
      "id": "string (container ID)",
      "name": "string (container name)"
    }
  ]
}
```

Notes:
- `subnets` and `containers` are always present; each is `[]` when the network has no subnets / no attached containers.
- `createdAt` is an RFC3339 timestamp.
- This endpoint is distinct from `GET .../networks/{networkName}/inspect`, which returns the raw, unmapped inspect report.

**Response (Error):**
- 400: `MISSING_NETWORK_NAME`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `NETWORK_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### DELETE /api/hosts/{id}/networks/{networkName}

Remove a network by exact name. Query flag `force` (default false): with
`force=true` podman removes the containers attached to the network —
stopping any that are running — and then removes the network. Without
`force` an attached container makes the removal a 409 `NETWORK_IN_USE`.
A non-boolean `force` value is ignored (treated as false), matching every
other boolean query flag in the API.

A remove failing at the call itself — the race window where the network
vanished or came into use between the request and podman's execution, no
per-network report involved — is classified the same way: not-found
answers 404 `NETWORK_NOT_FOUND`, an in-use refusal 409 `NETWORK_IN_USE`
carrying podman's message.

**Authentication:** Required

**Access:** Write level on `{id}`.

**Query Parameters:**
- `force` (boolean, optional, default false; non-boolean values are
  ignored and treated as false)

**Response (Success - 204):** no body.

**Response (Error):**
- 400: `MISSING_NETWORK_NAME`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row or read-level grantee)
- 404: `HOST_NOT_FOUND`, `NETWORK_NOT_FOUND`
- 409: `NETWORK_IN_USE` (a container is attached to the network and
  `force` was not set; carries podman's message)
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

### GET /api/hosts/{id}/networks/{networkName}/inspect

Return one network's raw podman inspect report — the full network configuration podman holds (subnets, routes, IPAM options, labels, attached containers), marshaled as-is with no DTO mapping. The report is the top-level JSON body.

**Authentication:** Required

**Access:** Any user with a host-access row for `{id}` (read level).

**Response (Success - 200):** the raw network inspect report as the top-level JSON body — podman's `NetworkInspectReport` shape (`name`, `id`, `driver`, `created`, `subnets`, `ipv6_enabled`, `internal`, `dns_enabled`, `labels`, `options`, `ipam_options`, `containers`), not the four-field list DTO.

**Response (Error):**
- 400: `MISSING_NETWORK_NAME`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `NETWORK_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`
