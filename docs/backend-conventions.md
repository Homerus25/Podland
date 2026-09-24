# Backend Conventions

Rules the reviewer enforces on `backend/` Go code, and pointers to the
reference implementations agents should mirror.

## Reference implementations

New domain endpoints should be written by reading and mirroring the
closest sibling that already exists. Rulings so far:

- **JSON endpoints (the endpoint envelope)** — `internal/web.Handle` is
  the seam every machine-scoped JSON endpoint crosses: it owns identity
  extraction, the AppError→envelope mapping, the 500 fallback, and JSON
  encode. The volumes handler (`internal/domain/volumes/handler.go`) is
  the reference: a handler is `web.Decode` + URL params + service call
  inside one `web.Handle` closure; identity/machine guards and error
  envelopes belong to the envelope and the subtree middleware, never to
  handlers. `web.Decode` (`internal/web`) owns the decode+validate
  ritual: a malformed body answers 400 `INVALID_REQUEST` ("Invalid
  request body"), a struct-tag failure 400 `VALIDATION_FAILED` with the
  validator's message — decode and validation errors belong to
  `web.Decode`'s vocabulary and are never hand-minted per handler. The
  same ruling covers route params and query flags: `web.URLParam` owns
  the missing-param guard (the error code stays explicit at the call
  site, the 400 message is derived from it) and
  `web.QueryBool`/`web.QueryInt` own the lenient flag parse (a missing
  or malformed value yields the explicit default) — route-param guards
  and query-flag parses belong to `web`'s vocabulary, never hand-minted
  per handler. The
  one body-bearing endpoint outside the envelope (images Pull, the
  pre-stream reference) responds the returned AppError itself before
  `streaming.Begin`.
  Every domain has migrated (volumes, containers, quadlets, secrets,
  images, machines subtree): the subtree handlers are envelope closures
  across the board, machines update/delete/status/access included.
  JSON endpoints outside the machine subtree (auth
  Login/Refresh/Logout/Status, users Create, machines Create/List)
  cross the user-scoped twin `web.HandleUser` (`internal/web`): it
  passes the context user ID through as-is — empty is a legal state
  (Status answers anonymous callers), the JWT middleware is the
  guarantor for the routes that need a caller, so there is no
  missing-identity invariant — and shares `Handle`'s error-mapping and
  success-write core, including the 204-skip. The auth handler
  (`internal/domain/auth/handler.go`) is the user-scoped reference:
  its body-bearing closures cross `web.Decode` like every other
  endpoint, and cookies are set or cleared inside the closure (headers
  only — the envelope owns the status line and body). The decode
  ritual is settled 2026-09-15 (architecture-review candidate C9).
- **NDJSON streaming endpoints (the streaming envelope)** —
  `streaming.Begin` + `streaming.Run` (`internal/streaming/`) are the
  single entry point for every NDJSON endpoint (container logs follow
  mode, quadlet logs follow mode, image pull, quadlet auto-update).
  `Begin` owns the NDJSON headers, writer creation (including the early
  500 `STREAMING_NOT_SUPPORTED`), and identity extraction — a missing
  identity there is the same wiring-bug ruling as `web.Handle`: 500
  `INTERNAL_ERROR` plus a loud log. `Run` owns the select loop,
  disconnect cancellation, the error→event mapping, and the
  exactly-one-terminal invariant — for log streams including `INFO`
  `STREAM_ENDED` on a clean end, for progress streams the domain's own
  complete event; the invariant is pinned once in the `streaming`
  package tests, not per domain. Handlers shrink to config plus event
  vocabulary: the config carries the domain's default error sentinel
  (`ErrorDefault`), the error-event builder receives already-mapped
  `(code, message, cancelled)` — never the raw error — and non-AppError
  causes are logged once by the module, never surfaced on the wire. Log
  streams use `Result = struct{}` and speak the shared terminal
  contract: `"CODE: message"` log events (`ERROR` level; `WARN` when
  cancelled). For log streams the kit owns the follow ritual:
  `streaming.RunLogStream` (`internal/streaming/logs.go`) is the
  reference, owning the terminal events (`INFO` `STREAM_ENDED` on a
  clean end, `"CODE: message"` errors) and the `ErrConnectionFailed`
  error default; a log handler is query parse + follow dispatch + two
  closures (the `LogStreamOp.Stream` callback and the buffered
  `web.Handle` service call). The kit owns the buffered fetch too:
  `streaming.FetchLogs` (same file) builds the podman logs options
  itself (`Follow=false`, `Timestamps=false`, the `Since`/`Tail` window
  through), installs the stdout/stderr writer pair, combines the
  output, and maps the failure mirroring `Run`'s terminal mapping:
  cancelled ctx → `OPERATION_CANCELLED`, else podman not-found → the
  op's caller-supplied sentinel, else one kit-owned
  `failed to get logs` wrap. A buffered log service is validate +
  engine acquisition + target resolution + the `FetchLogsOp`
  (containers and quadlets `GetLogs` are the references). The kit
  owns the follow fetch too: `streaming.FollowLogs` (same file)
  mirrors `FetchLogs` — podman logs options built once (`Follow=true`,
  `Timestamps=true` — the one intentional divergence, because the
  event writer parses the timestamps back out — with the `Since`/`Tail`
  window through), the stdout-INFO / stderr-ERROR event-writer pair
  installed, podman's not-found answer mapped to the op's
  caller-supplied sentinel, and any other cause carried by the same
  kit-owned log-only wrap. A follow log service is validate + engine
  acquisition + target resolution + the `FollowLogsOp` (containers
  and quadlets `GetLogsWithStream` are the references). Cancellation
  mapping lives nowhere below the kit: no service hand-copies
  `ctx.Err() == context.Canceled` — `Run`'s cancelled-wins decides for
  every streaming call, `FetchLogs` and `FollowLogs` for every log
  fetch. Under both sits the kit's abandonment contract,
  `streaming.Cancellable` (`internal/streaming/cancellable.go`): the
  engine runs on the connection cache's detached context (§4), so
  per-call cancellation is impossible below the streaming loop and the
  caller's only cancellation is abandoning the call — a pre-cancelled
  context answers `OPERATION_CANCELLED` without starting the work, a
  mid-call cancellation abandons the goroutine, which keeps running
  untracked on the detached context until it ends naturally or the
  connection is reaped (idle cleanup, LRU eviction, machine removal;
  release-while-in-use is the design, not a leak). Placement rule:
  `Cancellable` is called inside the `WithEngine` closure —
  abandonment only helps if it makes that closure return, and the
  closure returning is what releases the connection lease; abandoning
  at `Run`'s level would leave the closure blocked on the engine call
  with the lease pinned. `FetchLogs` and `FollowLogs` route their
  fetch through it, and images pull wraps exactly the engine pull call
  in it inside the closure (`PullWithProgress` in
  `internal/domain/images/service.go` is the reference — a real pull
  failure still maps to the image-pull-failed sentinel, the helper's
  sentinel passes through for `Run` to map). Quadlet auto-update is
  deliberately outside the contract: its seam is the SSH-exec stream,
  which takes the request context directly, so its own transport
  already honours cancellation. The
  kit's DTO vocabulary — the shared `LogEvent` and the
  `LogEventWriter` (RFC3339Nano parse, RFC3339 UTC re-emit,
  spaceless-line guard) — is pinned in `internal/streaming/logs.go`,
  and a log handler's query parse keeps its JSON-envelope fallback
  (issue 340's dispatch ruling). The
  images pull handler remains the reference for progress streams.
  Settled 2026-09-13 (architecture-review candidate C7); the buffered
  fetch settled 2026-09-18 (candidate C16); the follow fetch and the
  `Cancellable` abandonment contract settled 2026-09-22 (candidate
  C21).
- **Binary endpoints (the binary envelope)** — `web.HandleStream`
  (`internal/web`) is the seam every machine-scoped endpoint whose
  success is a byte stream or multipart upload rather than a DTO
  crosses. The envelope owns identity extraction (a missing identity is
  the same wiring-bug ruling as `web.Handle`: 500 `INTERNAL_ERROR` plus
  a loud log), AppError mapping (wrapped sentinels unwrapped via
  `errors.AsType`), and the 500 fallback with its log; the handler owns
  everything wire-specific — response headers, the body ritual, the
  service call, and the success write (implicit 200 for export,
  explicit 204 for populate). The volumes export/populate handler
  (`internal/domain/volumes/handler.go`, `Export`/`Populate`) is the
  reference. Settled 2026-09-15 (architecture-review candidate C8).
- **Handler contract tests** — `testutils.Perform` builds the
  request (identity, chi URL params, body) the way the router would;
  per-domain `recordXRequest` chi scaffolding is retired. Handler tests
  drive the **real domain service over the domain's fake engine** (the
  function-field fake in `engine_fake_test.go` behind its fake provider)
  — no service mock sits between handler and service. They pin only what
  the handler owns: routing, URL/query/body decoding, and the service's
  real behaviour through the HTTP surface. The envelope's own behaviour
  (5xx rewrite, missing-identity invariant, status+encode) is pinned once
  in `internal/web/web_test.go` (and `internal/streaming` for the
  streaming envelope), not per domain; a handler test whose only value
  was re-pinning that mapping is deleted, not ported.
- **Prune endpoints** — volumes prune is the reference pattern:
  `internal/domain/volumes/service.go` (`Prune`), `handler.go`
  (`Prune`), `dto.go` (`FromPodmanPruneReports`), and
  `internal/domain/volumes/handler_prune_test.go` plus
  `test/integration/volumes_test.go` (`Test_Integration_Volumes_Prune`).
- **Streaming progress (pull/logs)** — images pull is the reference:
  `internal/domain/images/handler.go` (`Pull`) and
  `internal/streaming/`.
- **Permission gating** — list/inspect at read level; anything that
  mutates remote state at write level via `r.With(requireWrite)` in
  `internal/app/app.go`. Bulk operations (prune) are write-level even
  if single-item deletes might be argued otherwise. The level contract
  itself — permission lookup fails ⇒ 500 `ErrInternalError`, stored
  level below the required one (an empty row included) ⇒ 403
  `ErrAccessDenied`, never a silent allow/deny guess — has one owner:
  `permission.RequireLevel` (`internal/permission`, with its
  `PermissionGetter` lookup interface). Its callers are the
  `RequireMachinePermission` middleware (which contributes the
  HTTP-shaped machine-resolution ritual — 401/400/404 — and maps the
  returned sentinels onto the error envelope the way the web envelope
  unwraps AppErrors) and the secrets reveal gate (admin level, fired
  before the connection lease). `models.HasAtLeast` and the level
  constants stay in `models`; `RequireAdmin` is a different domain
  (role, not level) with its own single owner already.
- **Runtime connection failures** — a persisted machine's podman socket
  unreachable during an operation is 502 `CONNECTION_FAILED`
  (`appErrors.ErrConnectionFailed`). The cause log (op, machineID,
  userID, cause) lives at the connection seam:
  `connections.Manager.WithConnection` warns and returns the sentinel;
  service call sites never map or log the acquisition error themselves.
  `ProbeMachine` has no `WithConnection` wrapper: the status endpoint logs
  the probe failure itself (the reason never surfaces to clients —
  machines can be shared with read-only users).
  Never a wrapped 500; `errors.go` documents the registration-vs-runtime
  distinction.
- **Connection seam (the podman prologue)** — every machine-scoped
  podman call crosses `connections.Manager`
  (`internal/connections/manager.go`); the volumes service is the
  reference for the shape (`internal/domain/volumes/service.go`, `List`):
  hold the result in a closure variable, call the domain's engine
  provider (`s.engines.WithEngine(ctx, userID, machineID, "domain.Op",
  func(e VolumeEngine) error)`) with an op tag, use the handed engine
  inside, return its error untouched. Each domain declares its own
  caller-width engine interface (no context parameter, single names,
  plain flags — the volumes engine in
  `internal/domain/volumes/engine.go` is the reference) plus a
  one-method `WithEngine` provider that delegates to
  `Manager.WithConnection`. The production adapter lives in one
  `engine.go` per domain package — the only file in the package
  importing the podman tunnel/bindings packages — and constructs the
  tunnel engine (and the package-function wrappers, e.g. quadlet
  install tar framing, volume export/import) inside the connection
  lease. Services keep their own request context for logging and
  disconnect detection; the engine never carries one. Service tests
  exercise real behaviour through a function-field fake engine
  (colocated in the domain's tests); an acquire failure is simulated by
  the fake provider returning `ErrConnectionFailed`. The machines
  domain keeps calling `connections.Manager` directly — ProbeMachine and
  TestConnection are the manager's other entry points. Machine deletion
  owns full teardown, wired in one place (`MachineService.Delete`):
  DB row removal first, then the manager's `RemoveMachine` (graceful,
  refcounted eviction of the cached podman connection), then the SSH-exec
  pool through the machines-local `sshInvalidator` seam (a single
  `RemoveMachine(machineID string)` method; the shared
  `sshexec.SSHExecInterface` is not widened, and `internal/app` passes
  the real sshexec service). Teardown runs only after a successful DB
  delete — a failed delete evicts nothing — and access rows ride the
  schema's `ON DELETE CASCADE`, so the delete path cleans them up only
  implicitly.

## Duplication across domains

A new endpoint that copies an established sibling pattern (error
envelope, `userID`/`machineID` extraction, JSON encode, test helper
like `recordPruneRequest`) is **correct**, not a smell. Extract shared
helpers only when a **third** domain copies the same shape. That
threshold exists to catch drift, not to count copies: when copies have
already diverged, extraction triggers even below the third copy — the
log-stream kit (`.scratch/380-log-stream-kit`) is the precedent, where
two timestamp parsers that disagreed on a spaceless podman line made
the streaming path un-changeable in one place. The threshold has been
crossed five times and all five extractions are now the reference: the
endpoint ritual became `internal/web.Handle`, the handler-test
scaffolding became `testutils.Perform`, the decode+validate ritual
became `web.Decode`, podman error classification became
`internal/podmanerr` — one leaf adapter owning podman's error model
(typed `ErrorModel` status first, phrase fallback over the lowercased
text) for volumes, secrets, containers, images, and quadlets — and the
param ritual became `web.URLParam`/`web.QueryBool`/`web.QueryInt`.
Domains classify through its predicates, check
most-specific first, and map the results to their own sentinels; the
adapter never imports `internal/errors`, so sentinels stay
domain-owned. Two vocabularies deliberately stay quadlet-local: the
quadlet running-refusal phrases (podman phrases that refusal outside
its shared conflict vocabulary) and lifecycle.go's systemctl/stderr
classification (a different transport). A second consumer for either
earns it a place in the adapter.

## API documentation sync

A change that alters what an endpoint accepts or returns — body fields,
status codes, error codes, cookie behavior — updates the matching
`doku/description_*_api.md` section in the same change. The doku file is
the contract; a handler change whose diff carries no matching doku hunk
is incomplete.

## Testing expectations

- Unit tests per domain package: handler tests against the real
  service over a per-domain function-field fake engine
  (`engine_fake_test.go`), plus service behaviour tests against the
  same fake engine — the handler-test pattern is the engine-seam shape
  and there is no per-domain service mock (see the handler contract
  tests ruling above). `internal/testutils` keeps
  `MockConnectionManager` for the machines domain, which still crosses
  the manager directly (its `RemoveMachine` records calls so service
  tests can pin deletion teardown), and
  `MockSSHExec` for the quadlets lifecycle's systemctl-over-SSH seam,
  plus the cancellation-promptness harness (`CancelStubs` +
  `RequirePromptReturnOnCancel`) — the third-copy extraction of the
  per-domain "returns promptly on cancel" regression tests (containers
  and quadlets logs follow, images pull). A domain pins only that its
  service releases the `WithEngine` closure on cancellation, through
  the harness; the abandonment contract itself stays pinned once in
  `internal/streaming`'s `Cancellable` suite.
- Integration tests live in `test/integration/` behind the
  `integration` build tag and follow the run-scoped-tag isolation
  model (see the header comment of `test/integration/images_test.go`).
  The suite runs against the **container test host**: a privileged
  Fedora container (image built by the `testhost-build`
  prerequisite of `make test-integration`; run contract owned by
  `test/integration/testhost.go`) that the harness boots and tears
  down per run, registering it through the same public API as any
  machine. Runs are serialized per host — fixed container name, fixed
  published port (127.0.0.1:24422), and cache volumes shared across
  runs — so a second concurrent run fails fast with "another
  integration run is active on this host"; that error is the
  serialization contract, not a broken environment. To target an
  externally managed host instead (e.g. a manually started podman
  machine on macOS), set the `PODLAND_TEST_SSH_*` overrides documented
  in the Makefile's `test-integration` header — the container
  lifecycle is skipped entirely.
- `go fmt ./...` skips build-tagged files; format them with
  `gofmt -w test/integration` when touched.
