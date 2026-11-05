# Podman Management API: Quadlet Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

---

## Quadlet Endpoints

### GET /api/hosts/{id}/quadlets

List all quadlets (Podman systemd unit files) on a host.

**Authentication:** Required

**Query Parameters:**
- `resource` (string, optional): Filter by resource type (container, pod, image, volume, network)
- `name` (string, optional): Filter by quadlet name

**Response (Success - 200):**
```json
[
  {
    "id": "string (uuid)",
    "name": "string",
    "path": "string",
    "unit": "string",
    "resource": "container|pod|image|volume|network",
    "state": "unknown|stopped|running|error",
    "raw": "string (optional, the raw quadlet file content)"
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

### GET /api/hosts/{id}/quadlets/{quadletId}

Get details of a specific quadlet including its content.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string (uuid)",
  "name": "string",
  "path": "string",
  "unit": "string",
  "resource": "container|pod|image|volume|network",
  "state": "unknown|stopped|running|error",
  "content": "string",
  "generation": integer,
  "createdAt": "ISO8601 timestamp",
  "updatedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/quadlets

Create a new quadlet on a host.

**Authentication:** Required

**Request:**
```json
{
  "name": "string (required)",
  "resource": "container|pod|image|volume|network (required)",
  "content": "string (required, the quadlet file content)"
}
```

**Response (Success - 201):**
```json
{
  "id": "string (uuid)",
  "name": "string",
  "path": "string",
  "unit": "string",
  "resource": "container|pod|image|volume|network",
  "state": "unknown|stopped",
  "content": "string",
  "createdAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `VALIDATION_FAILED`, `INVALID_REQUEST`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`
- 409: `QUADLET_ALREADY_EXISTS`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### PUT /api/hosts/{id}/quadlets/{quadletId}

Update an existing quadlet's content.

**Authentication:** Required

**Request:**
```json
{
  "content": "string (required, the updated quadlet file content)"
}
```

**Response (Success - 200):**
- Same as GET /api/hosts/{id}/quadlets/{quadletId}

**Response (Error):**
- 400: `VALIDATION_FAILED`, `INVALID_REQUEST`
- 401: `TOKEN_MISSING`, `TOKEN_AUTO_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### DELETE /api/hosts/{id}/quadlets/{quadletId}

Remove a quadlet from a host.

**Authentication:** Required

**Query Parameters:**
- `stop` (boolean, optional): Stop the resource before removing (default: true).
  When the resource is running and `force` is not set, the resource is stopped
  through the lifecycle's systemctl stop before the removal (a stop failure
  propagates and nothing is removed); with `stop=false` a running resource is
  refused with 400.
- `force` (boolean, optional): Force remove even if the resource is running
  (no explicit stop is performed).

**Response (Success - 204):**
- No body

**Response (Error):**
- 400: `QUADLET_RESOURCE_RUNNING` (if running and neither `force` nor `stop` is set)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/quadlets/{quadletId}/start

Start a stopped quadlet-managed resource.

**Authentication:** Required

**Response (Success - 200):**
```json
{
  "id": "string",
  "state": "running",
  "unit": "string"
}
```

**Response (Error):**
- 400: `QUADLET_ALREADY_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/quadlets/{quadletId}/stop

Stop a running quadlet-managed resource.

**Authentication:** Required

**Query Parameters:**
- `timeout` (integer, optional): Seconds to wait for the stop to finish. The
  backend wraps the systemctl call in the coreutils `timeout` command; when
  the wait expires the request answers 504 (the stop job may still complete
  in systemd afterwards — the error says the wait was given up, not that the
  stop failed).

There is no `force` parameter: systemctl's `--force` flag is documented for
enable/link/mask and the halt family only and is ignored for stop and
restart, so a force param would be a hollow affordance. A slow stop is
bounded by `timeout`; aggressive teardown stays with DELETE's `force`
(podman rm --force).

**Response (Success - 200):**
```json
{
  "id": "string",
  "state": "stopped",
  "unit": "string"
}
```

**Response (Error):**
- 400: `QUADLET_NOT_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`
- 504: `QUADLET_STOP_TIMED_OUT` (the timeout expired before the unit stopped)

---

### POST /api/hosts/{id}/quadlets/{quadletId}/restart

Restart a quadlet-managed resource.

**Authentication:** Required

**Query Parameters:**
- `timeout` (integer, optional): Seconds to wait for the whole restart to
  finish (one wrapped call — the restart is not split into a bounded stop
  followed by an unbounded start). Expiry answers 504.

No `force` parameter — same ruling as stop.

**Response (Success - 200):**
```json
{
  "id": "string",
  "state": "running",
  "unit": "string",
  "restartedAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `QUADLET_NOT_RUNNING`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`
- 504: `QUADLET_STOP_TIMED_OUT` (the timeout expired before the restart finished)

---

### GET /api/hosts/{id}/quadlets/{quadletId}/logs

Get logs for a quadlet-managed resource.

**Authentication:** Required

**Query Parameters:**
- `follow` (boolean, optional): Follow log output
- `since` (string, optional): Show logs since timestamp (RFC3339)
- `tail` (integer, optional): Number of lines to show from the end

**Response (Success - 200):**
```json
{
  "id": "string",
  "logs": "string (multiline log content)"
}
```

For streaming logs:
- **Content-Type**: `application/json`
- **Format**: Newline-delimited JSON (NDJSON) for streaming
- Each line is a log line object:

```json
{
  "timestamp": "ISO8601 timestamp",
  "level": "string (INFO, WARN, ERROR)",
  "message": "string"
}
```

The stream terminates with exactly one terminal event:

- A clean stream end emits `{"level": "INFO", "message": "STREAM_ENDED"}`.
- A failure emits an ERROR-level log-shaped event whose message embeds the
  error code, e.g. `"QUADLET_NOT_FOUND: Quadlet not found"` or
  `"CONNECTION_FAILED: Failed to connect to host"` — the same contract as
  the containers log stream, so clients parse it with the shared
  `"CODE: message"` heuristic.
- A client disconnect cancels the stream server-side and emits a WARN
  `OPERATION_CANCELLED` event (to the already-closed connection).

**Response (Error):**
- 400: `INVALID_SINCE_PARAM` / `INVALID_TAIL_PARAM` (malformed `since`/`tail`, message says "use RFC3339"; answered through the JSON envelope even when `follow=true`), `MISSING_QUADLET_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 404: `HOST_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR` (also for a miswired route missing identity), `STREAMING_NOT_SUPPORTED`
- 502: `CONNECTION_FAILED`
- 504: `OPERATION_CANCELLED` (buffered fetch cancelled by a client disconnect; follow-mode cancellation is the WARN terminal event above)

---

### GET /api/hosts/{id}/quadlets/{quadletId}/move-preview

Preview a quadlet move/copy to another host (issue 207): everything the
transfer dialog must show before confirming — the volumes the quadlet
references (whose data follows the file only when the transfer opts into
`migrateVolumeData`, issue 214), whether the target already has a quadlet
of the same name and resource, and whether the source quadlet's managed
resource is running. Volume entries whose source references a companion
`.volume` quadlet additionally carry that file and its state on the target
(issue 217).

**Authentication:** Required

Read-level access on the source host suffices (the endpoint mutates
nothing). The caller additionally needs **at least read access on the
target host**: the target's quadlet list is read server-side, and a
caller without target access gets the standard `ACCESS_DENIED` error —
never a preview. An unknown target host ID answers `ACCESS_DENIED`
the same way (no access row can exist for it); `HOST_NOT_FOUND` is
reserved for the source host in the path.

**Query Parameters:**
- `targetHostId` (string, required): ID of the target host the
  preview refers to.

**Response (Success - 200):**
```json
{
  "volumes": [
    {
      "source": "string",
      "kind": "volume|hostPath",
      "quadletFile": "string (optional, issue 217)",
      "targetState": "absent|identical|differs (optional, issue 217)",
      "podmanVolume": "string (optional, issue 214)",
      "sizeBytes": "number (optional, issue 214)",
      "existsOnTarget": "boolean (issue 214, always present on volume-kind entries)",
      "targetSizeBytes": "number (optional, issue 214)",
      "migratable": "boolean (issue 214, always present)",
      "reason": "string (optional, issue 214, present only when migratable is false)"
    }
  ],
  "secrets": [
    {
      "name": "string",
      "kind": "secret|hostPath",
      "existsOnSource": "boolean (always present; meaningful on secret-kind entries only)",
      "existsOnTarget": "boolean (always present; meaningful on secret-kind entries only)"
    }
  ],
  "collision": boolean,
  "running": boolean
}
```

- `volumes`: every `Volume=` line in the quadlet content, in file order.
  `source` is the volume source (the part of the value before the first
  `:`) and `kind` labels it: a source containing no path separator is a
  named podman volume (`"volume"`), anything else a host bind path
  (`"hostPath"`). A quadlet without `Volume=` lines reports an empty
  array.
- `quadletFile` / `targetState` (companion fields, issue 217, present
  together or not at all): a source ending in `.volume` with no path
  separator references the same-named quadlet FILE on the source host —
  exact filename match, bare names never match, podman resolves only the
  suffix form (the quadlet-generated volume is `systemd-<name>` unless
  `VolumeName=` overrides). When such a file exists on the source,
  `quadletFile` names it and `targetState` labels how it sits on the
  target: `"absent"` (the target has no such file), `"identical"`
  (byte-identical file already on the target — the transfer skips it), or
  `"differs"` (a same-name file with different content — the transfer
  refuses with 409). The comparison reads the target's file content
  through the connection seam, like the collision check. A `.volume`
  reference with no matching source file is dangling and stays an ordinary
  entry, as do bare names and host paths.
- `podmanVolume` (issue 214, volume-kind entries only): the podman volume
  the reference resolves to on BOTH hosts — a bare name is itself; a
  `.volume` reference resolves through its companion file's `VolumeName=`
  when the companion sets one, else podman's `systemd-<name>` default
  (dangling references still resolve the default). Host paths never
  resolve, so host-path entries omit it.
- `migratable` (issue 214, always present): whether `migrateVolumeData`
  can carry this reference's data — `true` for a volume-kind reference
  whose source volume exists; `false` for host bind paths (no podman
  export path) and dangling references (no source volume).
- `reason` (issue 214, present only when `migratable` is `false`): why the
  reference carries no data — `"host bind paths are not migrated"` or
  `"source volume does not exist"`.
- `sizeBytes` / `targetSizeBytes` / `existsOnTarget` (issue 214): the
  source and target volumes' content sizes when the engine can report one
  (podman's service API reports no volume usage today, so both are
  omitted), and whether the resolved volume already exists on the target
  (always present on volume-kind entries — `false` is meaningful, a fresh
  target).
- `secrets` (issue 215): every `Secret=` line in the quadlet content, in
  file order — the scan is type-generic, whatever the quadlet's resource
  type, and a quadlet without `Secret=` lines reports an empty array.
  `name` is the token before the first comma of the `Secret=` value; when
  that token is the all-options form (it contains `=`), the `source=`
  option governs, and an all-options token without `source=` names nothing
  and yields no entry. `kind` labels the resolved name: a podman secret
  (`"secret"`), or an absolute path (`"hostPath"`) that podman reads off
  the host filesystem at run time and that no secret store ever holds.
  `existsOnSource` / `existsOnTarget` answer whether the respective
  host's podman secret store holds the name (the target's read goes
  through the connection seam, like the collision check). Both booleans
  are always present but carry meaning only on secret-kind entries — a
  hostPath entry renders both `false`, the same no-claims shape as
  host-path volume entries. A secret-kind reference whose name is absent
  from the source store is an honest dangling entry (`existsOnSource:
  false`), never an error — the same convention as dangling volume
  references.
- `collision`: true exactly when the target host already has a quadlet
  of the same name and resource.
- `running`: the source quadlet's live state.

**Response (Error):**
- 400: `MISSING_TARGET_HOST_ID`, `MISSING_QUADLET_ID`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (no access row on the source, or below-read level on the target)
- 404: `HOST_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED` (source or target unreachable)

---

### POST /api/hosts/{id}/quadlets/{quadletId}/move

Transfer a quadlet to another host (issue 207). Two modes:

- **copy** (issue 210) installs the source quadlet's file — same name,
  resource, and content — on the target without overwriting and never
  touches the source: no stop, no removal, not even for a running quadlet.
  The `stop` / `force` flags are move-mode vocabulary and are ignored.
- **move** (issue 211) installs on the target exactly like copy, then
  removes the quadlet from the source — but only after the target install
  succeeded (the target's systemd manager reload included). The removal
  honors the delete flow's preconditions and flags (see below).

In both modes the installed unit is not started; startup stays the caller's
decision, exactly like creating a quadlet.

**Companion `.volume` quadlets (issue 217):** a `Volume=` source ending in
`.volume` with no path separator references the same-named quadlet file on
the source host (exact filename match; see the preview section's
discovery rule). When such companion files exist, the transfer carries
their definitions along and installs them on the target FIRST —
dependencies before the dependent — then the main quadlet:

- A companion already on the target **byte-identical is skipped**: nothing
  installs, no error, the transfer proceeds. This also makes re-running a
  partially failed transfer safe.
- A companion on the target with **different content fails 409**
  `QUADLET_ALREADY_EXISTS` (the message names the file) before anything
  further installs.
- The **first failure stops the set** with the standard error envelope
  naming the failed file; companions already installed stay on the target
  and the source is untouched — there is no rollback.
- Companions are **copy-only**: a move removes ONLY the main quadlet from
  the source. Companion files are shared definitions other source quadlets
  may reference. The volumes' DATA follows only when the request opts into
  `migrateVolumeData` (#214, below).
- `transferVolumeQuadlets: false` opts out: only the main quadlet
  transfers, and a referenced volume on the target then resolves to
  whatever the target already has.

**Volume data migration (issue 214):** `migrateVolumeData: true` opts the
transfer into carrying the referenced podman volumes' DATA along:

- Every volume-kind `Volume=` reference is resolved from the source
  content at execution time (bare name itself; `.volume` references
  through the companion's `VolumeName=`, else `systemd-<name>`; the set is
  deduplicated by resolved volume). Host bind paths never carry data.
- **Transport switches to NDJSON streaming**: instead of one JSON
  envelope, the response becomes a stream of newline-delimited events —
  per-volume progress, then exactly one terminal event. A file-only
  transfer (`migrateVolumeData` absent or false) answers the plain JSON
  envelope exactly as before. The stream carries its own 10-minute write
  deadline; a client disconnect cancels the operation and the stream ends
  with the terminal `OPERATION_CANCELLED` error event.
- **Phases run before anything is installed**: the target volumes are
  preflighted (an existing target volume holding content is refused with
  409 `VOLUME_MIGRATION_TARGET_NOT_EMPTY` — the message names the volume —
  unless `overwriteVolumes: true` opts into MERGING the imported data,
  podman import is merge, never replace; references whose source volume is
  absent are skipped, never probed or refused), then the secret transfer
  when `transferSecrets` opts into it (issue 215, below), then a move's
  stop, then the data itself. A data failure therefore answers the
  terminal error event with both hosts' quadlet sets untouched.
- **A migrating move stops before streaming**: a running source quadlet
  with `stop` (and not `force`) has its unit stopped BEFORE the data phase
  — exporting a volume under a live writer would capture torn state. The
  non-migrating move keeps the stop inside the removal as before. Any
  failure after that stop — a data-phase failure or a later install-phase
  refusal (`QUADLET_ALREADY_EXISTS`, an engine failure) — carries the stop
  note in the terminal error event's message: *"the source quadlet was
  stopped before this failure and remains stopped"* — no new error code;
  the source's runtime state was mutated even though the quadlet sets were
  not.
- Per-volume progress events (`type: "volume"`): `phase: "start"` when a
  volume's stream begins, `phase: "bytes"` as the streamed count crosses
  each MiB (carrying `bytes`), `phase: "done"` when the import finished
  (carrying the total `bytes`), `phase: "skip"` when the reference carried
  no data (a dangling reference; `reason` says why). All events share one
  `operationId`, the source `hostId`, and a `timestamp`.
- **Terminal events**: on success one `type: "complete"` event carrying
  the transfer envelope in `result` — including the additive
  `volumeMigration` block (present even when every reference was skipped,
  so skipped danglings stay visible). On failure one `type: "error"` event
  carrying `error: {code, message}`; the migration status codes ride this
  event instead of HTTP statuses (the stream itself answers 200):
  `VOLUME_MIGRATION_TARGET_NOT_EMPTY`, `VOLUME_UPLOAD_TOO_LARGE` (413
  semantics: the streamed data crossed `VOLUME_UPLOAD_MAX_BYTES` — the
  message names the volume and the byte count), `VOLUME_MIGRATION_FAILED`
  (an engine failure during the data phase — the message names the
  volume), plus the transfer's own codes (`QUADLET_ALREADY_EXISTS`,
  `ACCESS_DENIED`, `CONNECTION_FAILED`, ...).

**Secret transfer (issue 215):** `transferSecrets: true` opts the transfer
into carrying the referenced podman SECRETS — name and value — along:

- Every `Secret=` reference is resolved from the source content at
  execution time with the preview's naming rule (token before the first
  comma; the all-options form's `source=` governs) — the client never
  names secrets. The set is deduplicated by resolved name.
- Values move through the same reveal gate as the secrets API: the value
  is revealed on the source and the secret is created on the target with
  the identical value. The value is never persisted by the backend and
  never logged; the audit trail records only that a named secret moved
  between the two hosts.
- A secret-kind reference whose source secret does not exist (dangling),
  and a hostPath reference (read off the host filesystem at run time, no
  secret store involved), are **skipped and reported** in the response's
  `secretTransfer` block — never errors.
- A secret of the **same name already on the target is skipped BEFORE the
  reveal**, never overwritten — values are never compared, so a skip
  never means "equal". This makes re-running a partially failed transfer
  safe: secrets delivered earlier stay on the target and are skipped on
  the next run.
- Secrets are **copy-only**: a move never removes the source's secrets
  (other source quadlets may reference them), exactly like companion
  quadlet files.
- The secret phase runs **before anything is installed** — after the
  volume preflight but before a migrating move's stop and the volume data
  phase (cheap key-value transfers that need no stopped source, so their
  failures abort before any state is mutated or bytes move). A per-secret
  failure (a reveal or a create) aborts the transfer with 500
  `SECRET_TRANSFER_FAILED` (the message names the secret and the cause);
  a store-level failure answers the generic 500 — either way nothing is
  installed, nothing stopped, and both hosts' quadlet sets untouched.
- The transport does not change: secrets alone never switch the response
  to NDJSON streaming — only `migrateVolumeData` does (issue 214). With
  both flags set, the `secretTransfer` block rides the terminal
  `complete` event beside `volumeMigration`.

**Authentication:** Required

**Write-level access on the source host is required** (the transfer
mutates remote state through it). The caller additionally needs **at least
write access on the target host**: route middleware only sees the source
host, so the target's level is checked server-side against the access
store BEFORE any install — a read-level grantee (or a caller with no target
row) gets the standard `ACCESS_DENIED` and nothing is installed. An unknown
target host ID answers `ACCESS_DENIED` the same way (no access row can
exist for it); `HOST_NOT_FOUND` is reserved for the source host in
the path.

A `transferSecrets` request additionally requires **admin-level access on
the SOURCE host** (issue 215): moving secret values off a host is a
reveal, and the reveal gate owns value exposure — a below-admin caller
gets 403 `ACCESS_DENIED` in preflight, before anything is transferred or
installed. The target keeps its write-level requirement (a secret is
created on it).

A transfer onto the source's own host is not special-cased: it collides
with the quadlet itself and answers 409 with nothing installed.

**Move mode semantics:**

- A RUNNING source quadlet refuses the move with 400
  `QUADLET_RESOURCE_RUNNING` — before anything is installed — unless
  `stop` or `force` is set (the delete flow's precondition).
- `stop` (default `false`): a running source quadlet is stopped through
  the lifecycle seam before the removal (a stop-then-move). `force`
  (default `false`) is the alternative: the removal proceeds over the
  running resource, exactly like the delete flow's force.
- An install failure (or a target manager reload failure) answers a plain
  error and leaves the source untouched.
- A removal failure after a successful install answers 200 with the
  transfer envelope, `status: "partial"`, and the failure's detail in
  `error`: the quadlet then exists on BOTH hosts and the source side
  must be cleaned up manually. There is no rollback of the target install.
- A source quadlet that vanished between the install and the removal needs
  no removal — the answer is `status: "moved"`.

**Request:**
```json
{
  "targetHostId": "string (required)",
  "mode": "copy|move (required)",
  "stop": "boolean (optional, move mode only, default false)",
  "force": "boolean (optional, move mode only, default false)",
  "transferVolumeQuadlets": "boolean (optional, both modes, default true — issue 217)",
  "migrateVolumeData": "boolean (optional, both modes, default false — issue 214)",
  "overwriteVolumes": "boolean (optional, default false — issue 214, only read with migrateVolumeData)",
  "transferSecrets": "boolean (optional, both modes, default false — issue 215)"
}
```

**Response (Success - 200):**
```json
{
  "id": "string",
  "name": "string",
  "resource": "container|pod|image|volume|network",
  "status": "copied|moved|partial",
  "sourceHostId": "string",
  "targetHostId": "string",
  "path": "string (optional, the installed file's path on the target)",
  "error": "string (optional, present only when status is \"partial\")",
  "volumeMigration": {
    "volumes": [
      {
        "ref": "string",
        "podmanVolume": "string",
        "outcome": "migrated|skipped",
        "reason": "string (optional, skipped only)"
      }
    ]
  },
  "secretTransfer": {
    "secrets": [
      {
        "name": "string",
        "outcome": "transferred|skipped",
        "reason": "string (optional, skipped only)"
      }
    ]
  }
}
```

- `id` / `name`: the quadlet's file name, extension included — the transfer
  keeps the source's file name verbatim.
- `status`: `"copied"` for a completed copy, `"moved"` for a completed
  move, `"partial"` for a move whose target install succeeded but whose
  source removal failed (the quadlet exists on both hosts; see above).
- `error`: the removal failure's detail; present only with
  `"partial"`. Clients must narrate the both-hosts state from it.
- After a successful install the target host's systemd manager is
  reloaded (same guarantee as quadlet create); a reload failure answers
  `CONNECTION_FAILED` even though the file is installed — in move mode
  this fires before the source is touched at all.
- Companions are not itemized in the envelope: the main quadlet's fields
  speak for the transfer. The companion set is re-derived from the source
  content at execution time — the client never names files.
- `volumeMigration` (issue 214, present only when the request opted into
  `migrateVolumeData`, in both the plain envelope and the stream's
  terminal `complete` event — present even when every reference was
  skipped, so skipped danglings stay visible): the data phase's
  per-volume outcomes, file order, one entry per referenced volume-kind
  source (deduplicated by resolved podman volume). `ref` is the `Volume=`
  source as written, `podmanVolume` the podman volume it resolved to,
  `outcome` `"migrated"` (exported from the source, imported into the
  target) or `"skipped"` — with `reason` explaining a skip (a dangling
  reference has no source volume). Absent for file-only transfers.
- `secretTransfer` (issue 215, present only when the request opted into
  `transferSecrets`, in the plain envelope and in the stream's terminal
  `complete` event alike — present even when every reference was skipped,
  so skipped danglings and host paths stay visible): the secret phase's
  per-reference outcomes, file order, one entry per `Secret=` reference
  (deduplicated by resolved name). `name` is the resolved secret name,
  `outcome` `"transferred"` (revealed on the source, created on the
  target with the identical value) or `"skipped"` — with `reason`
  explaining a skip (`"a secret with this name already exists on the
  target"`, `"source secret does not exist"`, `"host path references are
  not transferred"`). Absent when the flag is not set.

**Response (Error):**
- 400: `MISSING_QUADLET_ID`, `INVALID_REQUEST` (malformed body),
  `VALIDATION_FAILED` (missing `targetHostId`, or a `mode` other than
  `"copy"` / `"move"`), `QUADLET_RESOURCE_RUNNING` (move mode: the source
  quadlet is running and neither `stop` nor `force` is set; nothing is
  installed)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED` (below-write level on the source, or below-write
  level on the target — checked before any install; issue 215: also a
  below-admin level on the source when `transferSecrets` is set — checked
  in preflight, before anything is transferred or installed)
- 404: `HOST_NOT_FOUND`, `QUADLET_NOT_FOUND`
- 409: `QUADLET_ALREADY_EXISTS` (the target already holds a quadlet of the
  same name and resource; nothing is installed or overwritten — the
  collision is re-checked at execution time. Also the companion refusal:
  the target already holds a companion `.volume` file with different
  content — the message names the file, nothing further installs, and
  companions already installed stay on the target)
- 413: `VOLUME_UPLOAD_TOO_LARGE` (issue 214: a migrating transfer's
  streamed volume data crossed `VOLUME_UPLOAD_MAX_BYTES` — the message
  names the volume and the byte count)
- 500: `INTERNAL_ERROR`, `VOLUME_MIGRATION_FAILED` (issue 214: an engine
  failure during the data phase — the message names the volume),
  `SECRET_TRANSFER_FAILED` (issue 215: a failure during the secret phase
  — nothing is installed. The sentinel's code and secret-naming message
  surface only on the stream's terminal error event when
  `migrateVolumeData` also set the transport; plain-envelope 5xx bodies
  are rewritten to the generic `INTERNAL_ERROR`)
- 502: `CONNECTION_FAILED` (source or target unreachable)

Issue-214 volume-migration refusals additionally carry `409
VOLUME_MIGRATION_TARGET_NOT_EMPTY` (a preflighted target volume holding
content; the message names the volume — `overwriteVolumes` opts into
merging instead). When `migrateVolumeData` puts the response into NDJSON
stream mode, ALL of these ride the terminal `type: "error"` event with
the stream itself answering 200 (above); the listed statuses apply to
the plain-envelope answers (file-only transfers, and errors answered
before the stream begins).

---

### POST /api/hosts/{id}/quadlets/{quadletId}/generate

Generate a quadlet file from an existing resource (container or pod).

**Authentication:** Required

**Request:**
```json
{
  "resourceId": "string (required)",
  "resourceType": "container|pod (required)"
}
```

**Response (Success - 200):**
```json
{
  "id": "string (uuid)",
  "name": "string",
  "path": "string",
  "unit": "string",
  "resource": "container|pod",
  "state": "unknown",
  "content": "string (generated quadlet content)",
  "createdAt": "ISO8601 timestamp"
}
```

**Response (Error):**
- 400: `INVALID_REQUEST`, `VALIDATION_FAILED`, `RESOURCE_NOT_FOUND`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`, `CONTAINER_NOT_FOUND`, `POD_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/hosts/{id}/quadlets/import

Import quadlets from uploaded content.

**Authentication:** Required

**Request:**
```json
{
  "quadlets": [
    {
      "name": "string (required, quadlet name without extension)",
      "resource": "container|pod|image|volume|network (required)",
      "content": "string (required, quadlet file content)"
    }
  ]
}
```

**Response (Success - 201):**
```json
{
  "imported": [
    {
      "id": "string (uuid)",
      "name": "string",
      "path": "string",
      "state": "unknown|stopped|running|error"
    }
  ],
  "failed": [
    {
      "name": "string",
      "error": "string"
    }
  ]
}
```

**Response (Error):**
- 400: `INVALID_REQUEST`, `VALIDATION_FAILED`
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`, `PERMISSION_DENIED`
- 404: `HOST_NOT_FOUND`
- 500: `INTERNAL_ERROR`
- 502: `CONNECTION_FAILED`

---

### POST /api/quadlets/convert-compose

Convert compose file content into quadlet entries (spec #336).

This endpoint is **global, not host-scoped**: it sits outside the
`/api/hosts/{id}` subtree, so there is no host ID and no host
access check — any authenticated user may call it, because conversion
touches no host data. The response carries the import endpoint's entry
shape, so the frontend can forward the body to
`POST /api/hosts/{id}/quadlets/import` unchanged.

Conversion runs `podlet compose` in default mode (no `--pod`/`--kube`) with
the compose content piped via stdin; relative host paths are left
untouched.

**Authentication:** Required (any authenticated user; no host access)

**Request:**
```json
{
  "content": "string (required, compose file content; the raw request body is capped at 1 MiB — 1,048,576 bytes)"
}
```

**Response (Success - 200):**
```json
{
  "quadlets": [
    {
      "name": "string (quadlet name without extension)",
      "resource": "container|pod|image|volume|network",
      "content": "string (quadlet file content)"
    }
  ]
}
```
Entries are sorted by name. The shape is exactly the import endpoint's
request body, so the response can be forwarded unchanged.

**Response (Error):**
- 400: `INVALID_REQUEST` (malformed JSON body, or a raw request body over the 1 MiB cap — the cap is enforced at the HTTP layer before decode, so an oversized body is never fully buffered), `VALIDATION_FAILED` (empty content; a genuine converter rejection — the converter started and exited non-zero, e.g. over a compose file using an unsupported option such as `build` together with `image` — answers `VALIDATION_FAILED` with the converter's stderr as the message — ANSI color escapes stripped, the text otherwise verbatim)
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 500: `INTERNAL_ERROR` (also when the converter binary is missing or unstartable — e.g. not on the server's PATH: the answer is the generic message, the raw exec error is logged server-side only, never on the wire)

---

### POST /api/hosts/{id}/quadlets/auto-update

Run `podman auto-update` on the host's quadlet-managed containers, streaming
the remote output live as NDJSON and terminating with a parsed report.

Only quadlets whose unit declares an update policy are visible to
`podman auto-update` — i.e. the `.container` file carries `AutoUpdate=registry`
(quadlet naming) and the unit is running. Policy-less quadlets are silently
skipped by podman.

**Permission level:** Write (read-level grantees receive 403 `ACCESS_DENIED`).

**Authentication:** Required

**Query Parameters:**
- `dryRun` (boolean, optional, default `false`): when `true`, passes
  `--dry-run` so podman checks for updates and reports what it *would* do
  without pulling or restarting anything. The report's `overall` is
  `dry-run` in that case.
- `filter` (string, optional, default empty): the systemd unit name to
  update, forwarded as `podman auto-update`'s positional argument; it
  composes with `dryRun`. Absent or empty updates every quadlet-managed
  container with an update policy, exactly as without the parameter; the
  opening plan event mirrors the narrowing — every participating unit, or
  only the filtered unit on a filtered run. The value is not validated —
  an unknown unit surfaces podman's own error through the stream.

**Response (Success - 200):** a live NDJSON stream (`Content-Type: application/json`),
one JSON event per line. Because registry pulls routinely exceed the global
write timeout, this endpoint extends its own response write deadline to
**10 minutes**; the stream stays open until the run finishes.

**Event schema** (all events share `operationId`, `hostId`, `timestamp`).
The stream opens with one plan event; the pulling events are additive and
interleave with the log events; the existing log, complete, and error
events are unchanged:

Plan event — the run's opening event, always exactly one, naming every
participating unit (the host's `.container` files carrying
`AutoUpdate=registry`) with its image, narrowed to the filtered unit on a
filtered run. A unit whose file is unreadable, or whose `Image=` is missing
or unparsable, appears as a unit-only entry (no `image`): the unit still
shows up, it just cannot be matched mid-run:
```json
{
  "operationId": "uuid",
  "type": "plan",
  "hostId": "host uuid",
  "plan": [
    { "unit": "myapp.service", "image": "docker.io/library/myapp:latest" },
    { "unit": "other.service" }
  ],
  "timestamp": "2026-11-01T10:00:00Z"
}
```

Pulling event — a planned unit entering the pulling state: one
`Trying to pull <image>` output line matched a planned unit's image, exact
match first, then suffix (podman prints normalized fully-qualified names;
quadlet files may carry short names). A suffix match counts only when the
character preceding the suffix in the pulled image is `/` (the
registry/repo boundary): a planned `app` matches
`docker.io/library/app:latest` but not `myapp:latest`. An image matching
no planned unit emits no pulling event. The pulling event precedes its
own log event:
```json
{
  "operationId": "uuid",
  "type": "pulling",
  "hostId": "host uuid",
  "unit": "myapp.service",
  "image": "docker.io/library/myapp:latest",
  "timestamp": "2026-11-01T10:00:02Z"
}
```

Log event — one line of remote `podman auto-update` output as it arrives:
```json
{
  "operationId": "uuid",
  "type": "log",
  "hostId": "host uuid",
  "stream": "stdout|stderr",
  "line": "human-readable podman output line",
  "timestamp": "2026-08-30T10:00:00Z"
}
```

Complete event — the terminal event on success, always exactly one, carrying
the parsed report:
```json
{
  "operationId": "uuid",
  "type": "complete",
  "hostId": "host uuid",
  "report": {
    "containers": [
      {
        "unit": "myapp.service",
        "containerName": "systemd-myapp",
        "imageName": "docker.io/library/myapp:latest",
        "updated": "updated|failed|pending|up-to-date",
        "error": "optional error text when updated is \"failed\""
      }
    ],
    "overall": "completed|completed-with-failures|dry-run|unparsable-report",
    "exitCode": 0
  },
  "timestamp": "2026-08-30T10:00:05Z"
}
```

Error event — the terminal event when the run fails before/instead of a
report (SSH failure, host unreachable, or cancellation):
```json
{
  "operationId": "uuid",
  "type": "error",
  "hostId": "host uuid",
  "error": {
    "code": "CONNECTION_FAILED|OPERATION_CANCELLED",
    "message": "human-readable message"
  },
  "timestamp": "2026-08-30T10:00:05Z"
}
```

**Cancellation semantics:** the run is bound to the request context — if the
client disconnects, the remote `podman auto-update` is aborted and the
host connection released. (The terminal event for the aborted run is
written to the already-closed connection, so clients observe cancellation
simply as the stream ending.)

**Response (Error):**
- 401: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`
- 403: `ACCESS_DENIED`
- 500: `INTERNAL_ERROR`, `STREAMING_NOT_SUPPORTED`
- 502: `CONNECTION_FAILED`
