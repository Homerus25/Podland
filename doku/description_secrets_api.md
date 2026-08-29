# Podman Management API: Secret Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

Secrets live only in the remote machine's podman; the backend proxies podman's service API and persists nothing locally. The permission model maps onto the machine levels:

- **read** — see secret **metadata** (name, ID, driver, timestamps, labels)
- **write** — additionally create (and, in a later slice, delete) secrets
- **admin** — additionally **reveal secret values** (`?reveal=true`)

**Redaction discipline:** a secret's value travels exactly twice — in the create request and in an admin reveal response. It is never persisted by the backend, never logged at any level, and never appears in any other response (list and metadata inspect responses have no value field at all).

---

## Secret Endpoints

### GET /api/machines/{id}/secrets

List the secrets on a machine. Metadata only: the response can never carry a value.

**Authentication:** Required

**Permission:** `read` (any grantee of the machine)

**Response (Success - 200):**
```json
[
  {
    "id": "string (podman secret ID)",
    "name": "string",
    "driver": "string (e.g. \"file\")",
    "createdAt": "ISO8601 timestamp",
    "updatedAt": "ISO8601 timestamp",
    "labels": "object (optional, omitted when empty)"
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

### GET /api/machines/{id}/secrets/{secretId}

Inspect a single secret. `{secretId}` may be the secret's name or its ID; podman resolves both.

**Authentication:** Required

**Permission:** `read` for a metadata inspect; `admin` when `?reveal=true` (a below-admin reveal is refused with 403 `ACCESS_DENIED`, not silently degraded to metadata, so the UI can distinguish "no permission" from "no value")

**Query Parameters:**
- `reveal` (boolean, optional): Ask for the secret's value. Defaults to `false`. Requires `admin`.

**Response (Success - 200):**
```json
{
  "id": "string (podman secret ID)",
  "name": "string",
  "driver": "string (e.g. \"file\")",
  "createdAt": "ISO8601 timestamp",
  "updatedAt": "ISO8601 timestamp",
  "labels": "object (optional, omitted when empty)",
  "secretData": "string (optional, present ONLY on an admin reveal)"
}
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row, or `?reveal=true` below admin)
- 404: `MACHINE_NOT_FOUND`, `SECRET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/machines/{id}/secrets

Create a secret on a machine. The value is forwarded to podman byte-identical — it is never trimmed or otherwise mutated, and leading/trailing whitespace and embedded newlines are significant.

**Authentication:** Required

**Permission:** `write` (the 403 fires in middleware before the request body — including the value — is processed)

**Request:**
```json
{
  "name": "string (required, max 255 characters, trimmed before validation)",
  "value": "string (required, 1..1048576 bytes)"
}
```

Validation rules:
- `name` must match podman's secret name rules: a letter or digit first, then letters, digits, underscores, dots, or dashes (`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`). Surrounding whitespace is trimmed before validation.
- `value` must be non-empty and at most 1 MiB (1048576 bytes). The cap is enforced in bytes, not characters. Whitespace-only values are valid — the whitespace is the secret.
- v1 creates use podman's default file driver with no labels (driver/driver-opts/labels are deferred).

**Response (Success - 201):**
```json
{
  "id": "string (podman secret ID)"
}
```

The response carries only the created ID — never the value.

**Response (Error):**
- 400: `VALIDATION_FAILED`, `INVALID_REQUEST`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `MACHINE_NOT_FOUND`
- 409: `SECRET_ALREADY_EXISTS` (a secret with this name already exists on the machine)
- 500: `INTERNAL_ERROR` (creation failures surface as `SECRET_CREATION_FAILED` internally; 5xx bodies are rewritten generic)
- 502: `CONNECTION_FAILED`

---

### DELETE /api/machines/{id}/secrets/{secretId}

Not yet implemented (planned slice 160-04). Podman secrets are immutable — an update is a conscious non-goal in v1; replacing a value means delete + recreate.
