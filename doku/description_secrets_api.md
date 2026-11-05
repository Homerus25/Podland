# Podman Management API: Secret Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

Secrets live only in the remote host's podman; the backend proxies podman's service API and persists nothing locally. The permission model maps onto the host levels:

- **read** — see secret **metadata** (name, ID, driver, timestamps, labels)
- **write** — additionally create and delete secrets
- **admin** — additionally **reveal secret values** (`?reveal=true`)

**Redaction discipline:** a secret's value reaches the client exactly twice — in the create request and in an admin reveal response. It is never persisted by the backend, never logged at any level, and never appears in any other response (list and metadata inspect responses have no value field at all). The one server-side exception is the quadlet move/copy transfer's opt-in secret phase (issue 215, see the [quadlets doc](description_quadlets_api.md)): a `transferSecrets` request reveals a referenced secret's value on the source — requiring **admin** on the source host, the same gate as `?reveal=true` — and recreates the secret, value identical, on the target; the value never reaches the client, and each moved secret is audit-logged by name only. The standalone secret copy/move (issue 239, below) is the second server-side exception with the same shape: the transfer reveals the value on the source (admin) and recreates it on the target; the value never reaches the client and the audit event carries the name only.

---

## Secret Endpoints

### GET /api/hosts/{id}/secrets

List the secrets on a host. Metadata only: the response can never carry a value.

**Authentication:** Required

**Permission:** `read` (any grantee of the host)

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
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/hosts/{id}/secrets/{secretId}

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
- 404: `HOST_NOT_FOUND`, `SECRET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### GET /api/hosts/{id}/secrets/{secretId}/inspect

Inspect a single secret and answer with podman's **raw inspect report** as the top-level JSON body (issue 287) — the entity's own field names (`ID`, `CreatedAt`, `UpdatedAt`, `Spec`), not the mapped DTO the `GET .../secrets/{secretId}` endpoint returns. `{secretId}` may be the secret's name or its ID; podman resolves both.

**Redaction guarantee:** the response **never contains the secret's value**. The endpoint never passes reveal to podman (no `showsecret`), so the report's `SecretData` field is omitted entirely. There is no `?reveal=` parameter and no admin gate on this endpoint — metadata is visible to any grantee at read level. The value still reaches the client exactly twice: the create request and an admin reveal response (`GET .../secrets/{secretId}?reveal=true`).

**Authentication:** Required

**Permission:** `read` (any grantee of the host)

**Response (Success - 200):**
```json
{
  "ID": "string (podman secret ID)",
  "CreatedAt": "ISO8601 timestamp",
  "UpdatedAt": "ISO8601 timestamp",
  "Spec": {
    "Name": "string",
    "Driver": { "Name": "string (e.g. \"file\")", "Options": "object (or null)" },
    "Labels": "object (or null when the secret has none)"
  }
}
```

`SecretData` is always omitted — the endpoint never asks podman for the value.

**Response (Error):**
- 400: `MISSING_SECRET_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `SECRET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/secrets

Create a secret on a host. The value is forwarded to podman byte-identical — it is never trimmed or otherwise mutated, and leading/trailing whitespace and embedded newlines are significant.

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
- 404: `HOST_NOT_FOUND`
- 409: `SECRET_ALREADY_EXISTS` (a secret with this name already exists on the host)
- 500: `INTERNAL_ERROR` (creation failures surface as `SECRET_CREATION_FAILED` internally; 5xx bodies are rewritten generic)
- 502: `CONNECTION_FAILED`

---

### DELETE /api/hosts/{id}/secrets/{secretId}

Removes a secret from the host. The `{secretId}` path segment accepts a secret name or ID. Gated at **write** level: `read` → 403, `write`/`admin` → proceed.

**Responses**

- 204: removed successfully. The response body is **empty** — the frontend (series 170) should invalidate its secrets list on the 204, there is nothing to parse.
- 403: `ACCESS_DENIED` (caller below write level)
- 404: `SECRET_NOT_FOUND` (no such secret on the host)
- 409: `SECRET_IN_USE` (podman refuses to remove a secret referenced by a container; podman's own refusal message — naming the container that holds the secret — is forwarded in the 409 body since it is a 4xx, not rewritten generic)
- 500: `INTERNAL_ERROR` (other podman failures; 5xx bodies are rewritten generic)
- 502: `CONNECTION_FAILED`

Podman secrets are immutable — an update is a conscious non-goal in v1; replacing a value means delete + recreate.

---

### GET /api/hosts/{id}/secrets/{secretId}/move-preview

Preview a secret copy/move to another host (issue 239): the facts the
transfer dialog must show before confirming — whether the target host
already holds a same-named secret, and which source quadlets reference it.
The `{secretId}` path segment accepts a secret name or ID.

**Authentication:** Required

Read-level access on the source host suffices (the endpoint mutates
nothing and reveals nothing). The caller additionally needs **at least
read access on the target host**: the target's secret store is read
server-side, and a caller without target access gets the standard
`ACCESS_DENIED` error — never a false preview. An unknown target host
ID answers `ACCESS_DENIED` the same way (no access row can exist for it);
`HOST_NOT_FOUND` is reserved for the source host in the path.

**Query Parameters:**
- `targetHostId` (string, required): ID of the target host the
  preview refers to.

**Response (Success - 200):**
```json
{
  "collision": boolean,
  "referencingQuadlets": [ { "name": "string" } ]
}
```

- `collision`: true exactly when the target host already holds a
  secret of the same name. The transfer never overwrites — the flag
  drives the dialog's confirm gate, and the transfer re-checks at
  execution time.
- `referencingQuadlets`: the source host's quadlet files whose
  `Secret=` lines resolve to the secret — the token before the first
  comma, unless it carries `=` (the all-options form, where `source=`
  governs); absolute-path sources are host-path references and never
  match. A warning, never a block; quadlets are not transferred by a
  secret copy/move. Empty is `[]`, never `null`.

**Response (Error):**
- 400: `MISSING_SECRET_ID`, `MISSING_TARGET_HOST_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row on the source, or below-read level
  on the target)
- 404: `HOST_NOT_FOUND`, `SECRET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED` (source or target unreachable)

---

### POST /api/hosts/{id}/secrets/{secretId}/move

Transfer a secret to another host (issue 239). The `{secretId}` path
segment accepts a secret name or ID; the name is preserved on both
hosts — no rename, no overwrite. Two modes:

- **copy** reveals the value on the source and creates the secret on the
  target, value byte-identical; the source is never touched.
- **move** does the same, then removes the source secret — only after
  the target holds it (install-first-then-remove). A failed source
  removal does not fail the request: the envelope answers `status:
  "partial"` with the failure's detail in `error` — the secret then
  exists on both hosts and the source side must be cleaned up
  manually. A source secret that vanished between the transfer and the
  removal needs no removal — the answer is `"moved"`.

**Transport is the plain JSON envelope** — no NDJSON stream: secret
values are small, the streaming transport stays a volume concern. The
value never appears in the response; it travelled source→target inside
the transfer.

**Permission:** the route's write-level middleware proves the caller may
mutate the SOURCE host, but the value LEAVES it — a reveal — so the
service gates the source at **admin** (the reveal gate's own level; a
below-admin caller gets 403 `ACCESS_DENIED` and podman is never asked
for the value). The TARGET host needs **at least write access**,
enforced in-service against the access store (route middleware only sees
the route's host). The operation is therefore effectively
admin-on-source plus write-on-target.

**The collision is re-checked at execution time**: a target secret that
appeared since the preview answers 409 `SECRET_ALREADY_EXISTS` — never
an overwrite.

**Request:**
```json
{
  "targetHostId": "string (required)",
  "mode": "string (required, \"copy\" or \"move\")"
}
```

**Response (Success - 200):**
```json
{
  "name": "string",
  "status": "copied|moved|partial",
  "sourceHostId": "string",
  "targetHostId": "string",
  "error": "string (optional, present only when status is \"partial\")"
}
```

- `error`: the source removal failure's detail; present only with
  `"partial"`. Clients must narrate the both-hosts state from it.

The transfer is audit-logged value-free ("secret transferred": who, from
which host, to which host, by name — never the value).

**Response (Error):**
- 400: `MISSING_SECRET_ID`, `INVALID_REQUEST` (malformed body),
  `VALIDATION_FAILED` (missing `targetHostId`, or a `mode` other than
  `"copy"` / `"move"`)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (below-admin level on the source, or below-write
  level on the target)
- 404: `HOST_NOT_FOUND`, `SECRET_NOT_FOUND` (the source secret
  vanished before the reveal)
- 409: `SECRET_ALREADY_EXISTS` (a same-named secret on the target — the
  execution-time re-check)
- 500: `INTERNAL_ERROR`, `SECRET_TRANSFER_FAILED` (an engine failure
  during the reveal→create phase)
- 502: `CONNECTION_FAILED` (source or target unreachable)
