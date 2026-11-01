# Podman Management API: Machine Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

---

## Machine Endpoints

### POST /api/machines

Register a new remote machine.

**Authentication:** Required

**Request:**
```json
{
  "name": "string (required)",
  "description": "string (optional)",
  "hostname": "string (required)",
  "port": "integer (optional, default: 22)",
  "sshUsername": "string (required)",
  "sshPrivateKey": "string (required, PEM format)",
  "podmanSocketPath": "string (optional, default: '/run/podman/podman.sock')"
}
```

**Validation:**
- `name` must be unique per user
- `hostname` must be valid (DNS or IP)
- `sshPrivateKey` must be valid PEM format
- Backend will **test the SSH connection** before saving

**Response (Success - 201):**
```json
{
  "id": "string (uuid)",
  "name": "string",
  "description": "string",
  "hostname": "string",
  "port": 22,
  "sshUsername": "string",
  "sshKeyFingerprint": "string",
  "podmanSocketPath": "/run/podman/podman.sock",
  "ownerId": "string (uuid)",
  "createdAt": "ISO8601 timestamp",
  "updatedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed JSON body), `VALIDATION_FAILED`, `INVALID_SSH_KEY`, `CONNECTION_FAILED`, `CONNECTION_TEST_TIMEOUT`, `MACHINE_ALREADY_EXISTS`, `HOST_NOT_ALLOWED`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 500: `INTERNAL_ERROR`

**Error messages (registration steps name themselves):** the 4xx messages name the failed registration step; the error codes stay stable. Clients may render the message verbatim.

- `VALIDATION_FAILED` lists the missing field names: `"Missing required fields: name, hostname"`
- `INVALID_SSH_KEY` describes the expected format: a PEM block, i.e. a `-----BEGIN OPENSSH PRIVATE KEY-----` (or RSA/EC/DSA/ED25519) header line paired with its matching `-----END ... PRIVATE KEY-----` footer line
- `CONNECTION_FAILED` is a STATIC message: `"Failed to connect to machine during registration — check the hostname, port, SSH username, private key, and podman socket path"`. No probe detail (dial error text, target address, open/closed distinction) reaches the client — the registration probe dials an arbitrary user-supplied hostname:port from the server's network vantage point, so the raw error is an internal port-scan/SSRF oracle (pentest 2026-11-01, finding 1). The full underlying error is logged server-side with the request ID. The residual pure timing oracle (fast-fail vs `CONNECTION_TEST_TIMEOUT`) is accepted.
- `CONNECTION_TEST_TIMEOUT` states the exceeded bound: `"Connection test timed out after 10s"` (the configured `MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS`)
- `MACHINE_ALREADY_EXISTS` names the taken machine name: `"Machine with name \"my-box\" already exists"`
- `HOST_NOT_ALLOWED` (only when `MACHINE_HOST_BLOCK_PRIVATE_RANGES=true`): the hostname was rejected by the instance's egress block before any dial — an IP literal checked directly, or a hostname through every resolved IP, falling in loopback (127.0.0.0/8, ::1), RFC1918 (10/8, 172.16/12, 192.168/16), link-local (169.254/16 — including the 169.254.169.254 cloud-metadata endpoint — and fe80::/10), CGNAT (100.64/10), IPv6 ULA (fc00::/7), or 0.0.0.0/8; an unresolvable hostname fails closed with the same answer. Message: `"Machine hostname is not allowed: this instance blocks private, loopback, link-local, CGNAT, and unspecified addresses"`

A failed registration probe is logged server-side with the underlying error and the request ID. 5xx failures (database persist, access grant, key encryption) keep the generic internal-error message — no step naming that leaks internals.

---

### GET /api/machines

List all machines the user has access to.

**Authentication:** Required

**Query Parameters:**
- None

**Response (Success - 200):**
```json
[
  {
    "id": "string (uuid)",
    "name": "string",
    "description": "string",
    "hostname": "string",
    "port": 22,
    "sshUsername": "string",
    "sshKeyFingerprint": "string",
    "podmanSocketPath": "/run/podman/podman.sock",
    "ownerId": "string (uuid)",
    "permission": "read|write|admin",
    "isOwner": true,
    "createdAt": "ISO8601 timestamp",
    "updatedAt": "ISO8601 timestamp"
  }
]
```

- `permission`: the caller's own permission level on the machine
- `isOwner`: `true` exactly when the caller is the machine's owner; present on every row, so a grantee reads an explicit `false`

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 500: `INTERNAL_ERROR`

---

### GET /api/machines/{id}

Get details of a specific machine.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string (uuid)",
  "name": "string",
  "description": "string",
  "hostname": "string",
  "port": 22,
  "sshUsername": "string",
  "sshKeyFingerprint": "string",
  "podmanSocketPath": "/run/podman/podman.sock",
  "ownerId": "string (uuid)",
  "permission": "read|write|admin",
  "isOwner": true,
  "createdAt": "ISO8601 timestamp",
  "updatedAt": "ISO8601 timestamp"
}
```

The detail response carries the same per-caller enrichment as the list endpoint:
- `permission`: the caller's own permission level on the machine
- `isOwner`: `true` exactly when the caller is the machine's owner; never omitted, so a grantee reads an explicit `false`

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### GET /api/machines/{id}/status

Perform a live status check of a machine. The machine is online when the SSH connection to the host succeeds AND the Podman service API answers its ping over that connection; anything else — dial failure, auth failure, missing socket, ping failure, or timeout — is reported as `offline`. There is no third server-side state. A cached per-machine connection is reused when present; a full SSH dial only happens when cold. The probe is bounded by the same timeout regime as the registration connection test (`MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS`, 10 seconds default). Any user with access may check status (read level suffices). Connection error details are never included in the response; they are logged server-side only, because SSH-level errors describe the owner's infrastructure and machines can be shared.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "status": "online",
  "checkedAt": "2026-08-29T12:00:00Z"
}
```

- `status`: `"online"` or `"offline"` (offline is a valid answer, not an error)
- `checkedAt`: RFC3339 UTC timestamp

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### GET /api/machines/{id}/version

Ask the remote Podman service API for its `/version` report over the cached per-machine connection (the same acquisition seam as every other machine-scoped podman call) and answer the server component's `Version` and `APIVersion`. The versions are a live remote fact, not stored data. Any user with access may call the endpoint (read level suffices, same as status).

Unavailable is a valid answer, not an error: when the connection or the version call fails, the endpoint answers 200 with both fields `null`. The failure is logged server-side with the request ID; connection-level detail never reaches the response, because SSH-level errors describe the owner's infrastructure and machines can be shared with read-only users (the status endpoint's rule).

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "podmanVersion": "5.4.1",
  "apiVersion": "5.4.1"
}
```

- `podmanVersion`: the remote Podman server component's version, or `null` when it could not be asked
- `apiVersion`: the remote Podman service API's version, or `null` when it could not be asked

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### PUT /api/machines/{id}

Update an existing machine's connection details.

**Authentication:** Required (owner only)

**Request:**
```json
{
  "name": "string (optional)",
  "description": "string (optional)",
  "hostname": "string (optional)",
  "port": "integer (optional)",
  "sshUsername": "string (optional)",
  "sshPrivateKey": "string (optional, PEM format)",
  "podmanSocketPath": "string (optional)"
}
```

Only the owner may update a machine; any other user with an access row (read or write) receives 403 `ACCESS_DENIED`.

**Note:** If `sshPrivateKey` is provided, the backend will test the new connection before persisting: the probe targets the resulting merged connection configuration (request fields override stored ones), a failure answers 400 `CONNECTION_FAILED`, and a probe timeout answers 400 `CONNECTION_TEST_TIMEOUT`. Updates that do not change the key skip the probe. The update path shares the registration probe, so its 4xx messages carry the same wording as POST /api/machines (the static message on `CONNECTION_FAILED`, the exceeded bound on `CONNECTION_TEST_TIMEOUT`, the PEM format description on `INVALID_SSH_KEY`).

When `MACHINE_HOST_BLOCK_PRIVATE_RANGES=true`, a request that sets `hostname` — with or without a new SSH key — is policy-checked before the probe and before anything is persisted: a blocked target (see POST /api/machines) answers 400 `HOST_NOT_ALLOWED` and is never dialed, never stored. Updates that do not touch `hostname` skip the check.

**Response (Success - 200):**
- The machine fields of GET /api/machines/{id} without the per-caller `permission` and `isOwner` fields (only the list and detail GET responses carry the enrichment)

**Response (Error):**
- 400: `INVALID_REQUEST`, `INVALID_SSH_KEY`, `CONNECTION_FAILED`, `CONNECTION_TEST_TIMEOUT`, `HOST_NOT_ALLOWED`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### DELETE /api/machines/{id}

Remove a machine.

**Authentication:** Required

**Response (Success - 204):**
- No body

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### GET /api/machines/{id}/access

List users with access to a machine.

**Authentication:** Required

**Response (Success - 200):**
```json
[
  {
    "userId": "string (uuid)",
    "username": "string",
    "permission": "read|write|admin",
    "grantedAt": "ISO8601 timestamp"
  }
]
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### POST /api/machines/{id}/access

Grant another user access to a machine.

**Authentication:** Required (must be owner or have admin permission)

**Request:**
```json
{
  "userId": "string (uuid, required)",
  "permission": "read|write|admin (required)"
}
```

**Response (Success - 201):**
```json
{
  "machineId": "string (uuid)",
  "userId": "string (uuid)",
  "permission": "read|write|admin"
}
```

**Response (Error):**
- 400: `INVALID_REQUEST`, `INVALID_PERMISSION`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `USER_NOT_FOUND`
- 500: `INTERNAL_ERROR`

---

### DELETE /api/machines/{id}/access/{userId}

Revoke a user's access to a machine.

**Authentication:** Required (must be owner or have admin permission)

**Response (Success - 204):**
- No body

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `MACHINE_NOT_FOUND`, `USER_NOT_FOUND`
- 500: `INTERNAL_ERROR`
