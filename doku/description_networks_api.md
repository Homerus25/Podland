# Podman Management API: Network Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

Networks live only in the remote machine's podman; the backend proxies podman's service API and persists nothing locally. The networks surface is read-only: listing is available at `read` level, and there are no create/delete/prune endpoints (a follow-up issue).

---

## Network Endpoints

### GET /api/machines/{id}/networks

List every network on a machine — everything podman reports, including the default bridge network. No filtering, no built-in marking; the list reflects the machine's real state.

**Authentication:** Required

**Access:** Any user with a machine-access row for `{id}` (read level).

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
- 404: `MACHINE_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`
