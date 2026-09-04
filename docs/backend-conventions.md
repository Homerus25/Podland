# Backend Conventions

Rules the reviewer enforces on `backend/` Go code, and pointers to the
reference implementations agents should mirror.

## Reference implementations

New domain endpoints should be written by reading and mirroring the
closest sibling that already exists. Rulings so far:

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

## Duplication across domains

A new endpoint that copies an established sibling pattern (error
envelope, `userID`/`machineID` extraction, JSON encode, test helper
like `recordPruneRequest`) is **correct**, not a smell. Extract shared
helpers only when a **third** domain copies the same shape.

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
