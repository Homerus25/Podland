---
title: Volumes migrate to WithConnection (reference slice)
parent: Deepen the connection stack — one ConnectionManager seam with real teardown
date: 2026-09-07
status: ready-for-agent
labels:
  - connections
  - volumes
  - backend
  - refactor
author: vibe-agent
---

## Parent

Deepen the connection stack (see `.scratch/230-deepen-connection-stack/issue.md`).

## What to build

`VolumeService` depends on `connections.Manager` instead of the old interface, and every method (list, inspect, create, delete, prune, export, populate) wraps its podman engine call in `WithConnection`. The call-site shape — closure capture, settled in the design session:

```go
func (s *VolumeService) List(ctx context.Context, userID, machineID string, opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error) {
	var volumes []*entities.VolumeListReport
	err := s.conn.WithConnection(ctx, userID, machineID, "volumes.List", func(connCtx context.Context) error {
		engine := &tunnel.ContainerEngine{ClientCtx: connCtx}
		v, err := engine.VolumeList(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to list volumes: %w", err)
		}
		volumes = v
		return nil
	})
	return volumes, err
}
```

- The `op` string is the log label (`"volumes.List"`, `"volumes.Prune"`, …).
- Call sites **stop** mapping errors to `ErrConnectionFailed` and stop warn-logging the cause — the seam owns both now. Domain-specific error mapping (not-found, in-use) stays in the service, unchanged.
- `MockConnectionManager` in `testutils` gains `WithConnection` (runs `fn` with a mock context, can inject an acquisition error) and `RemoveMachine` (no-op); the old mock methods stay until the contract ticket.
- Volumes service tests: delete the per-method "connection error → ErrConnectionFailed" pins (the mapping lives behind the seam and is pinned once in `connections`); everything else stays. Handler contract tests and the integration suite are untouched — the response envelope is identical, so **no doku API hunks**.
- This slice is the **reference implementation** the sweep tickets copy, the same way volumes prune is the prune reference.

## Acceptance criteria

- [ ] No `GetConnection`/`Release` references remain anywhere in the volumes domain
- [ ] All volumes unit tests, handler contract tests, and `go test ./...` green
- [ ] Volumes service tests contain no connection-error mapping pins
- [ ] Behavior identical from the API: 502 `CONNECTION_FAILED` envelope, error codes, and success responses unchanged

## Blocked by

- `230-deepen-connection-stack-02-deep-connection-manager`
