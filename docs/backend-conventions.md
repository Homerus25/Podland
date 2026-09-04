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
  the reference: a handler is decode + URL params + service call inside
  one `web.Handle` closure; identity/machine guards and error envelopes
  belong to the envelope and the subtree middleware, never to handlers.
  Domains not yet migrated (images, machines subtree) still carry the
  old ritual; migrate them domain-by-domain, machines last.
- **Handler contract tests** — `testutils.Perform` builds the
  request (identity, chi URL params, body) the way the router would;
  per-domain `recordXRequest` chi scaffolding is retired. The envelope's
  own behaviour (5xx rewrite, missing-identity invariant, status+encode)
  is pinned once in `internal/web/web_test.go`, not per domain.
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
  if single-item deletes might be argued otherwise.
- **Runtime connection failures** — a persisted machine's podman socket
  unreachable during an operation is 502 `CONNECTION_FAILED`
  (`appErrors.ErrConnectionFailed`), cause logged at the service site.
  Never a wrapped 500; `errors.go` documents the registration-vs-runtime
  distinction.

## Duplication across domains

A new endpoint that copies an established sibling pattern (error
envelope, `userID`/`machineID` extraction, JSON encode, test helper
like `recordPruneRequest`) is **correct**, not a smell. Extract shared
helpers only when a **third** domain copies the same shape. That
threshold has been crossed twice so far and both extractions are now
the reference: the endpoint ritual became `internal/web.Handle`, and
the handler-test scaffolding became `testutils.Perform`.

## API documentation sync

A change that alters what an endpoint accepts or returns — body fields,
status codes, error codes, cookie behavior — updates the matching
`doku/description_*_api.md` section in the same change. The doku file is
the contract; a handler change whose diff carries no matching doku hunk
is incomplete.

## Testing expectations

- Unit tests per domain package: handler contract tests against a
  testify mock (`Mock*Service`), plus service tests against the mock
  connection manager (`internal/testutils`).
- Integration tests live in `test/integration/` behind the
  `integration` build tag, follow the run-scoped-tag isolation model
  (see the header comment of `test/integration/images_test.go`), and
  run against a real podman machine.
- `go fmt ./...` skips build-tagged files; format them with
  `gofmt -w test/integration` when touched.
