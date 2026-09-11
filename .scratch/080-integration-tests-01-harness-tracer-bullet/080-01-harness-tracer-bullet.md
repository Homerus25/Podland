---
title: Integration harness + tracer-bullet test
parent: 080-integration-tests
date: 2026-08-11
status: ready-for-agent
labels:
  - ready-for-agent
  - testing
  - backend
  - integration-tests
  - podman-integration
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

The integration test harness in `backend/test/integration/` (every file carrying `//go:build integration`) plus **one** end-to-end test that proves the whole pipeline. This is the canonical tracer bullet: machine lifecycle → discovery → in-process backend → JWT/Bearer auth → registration (encryption-at-rest) → SSH tunnel → podman socket → back to HTTP.

`TestMain` performs, in order:

1. **Ensure machine.** `podman machine start $PODLAND_TEST_MACHINE_NAME` (default `podman-machine-default`). Install a SIGINT/SIGTERM handler so teardown runs even on interrupt.
2. **Discover target.** Parse `podman system connection list` for the connection matching the machine name → host/port/ssh-user/in-VM socket path + Identity key path. Each field overridable via `PODLAND_TEST_SSH_{HOST,PORT,USER,SOCKET,KEY_PATH}` (escape hatch to target `pi` or any host).
3. **Bring up backend in-process.** `app.New(testCfg)` where `testCfg` uses a temp SQLite file, temp JWT PEM files (via `testutils.CreateTestJWTKeys`), and a random `ENCRYPTION_KEY`; wrap `app.Handler()` in `httptest.NewServer`.
4. **Seed user + real login.** bcrypt-insert one user (mirror `cmd/initdb`); `POST /api/auth/login`; capture token from the JSON body.
5. **Register machine once.** `POST /api/machines` as the seeded user with the discovered connection details + key material; capture the returned machine ID; share across tests.
6. **Readiness.** Poll `podman machine inspect` until `State==running`, then retry a real podman-over-SSH probe (e.g. the tracer GET below, or `podman --connection <name> ps`) with exponential backoff, ~60s budget.
7. **Teardown.** `httptest` server close → `app.Close()` → delete temp files → `podman machine stop`. The signal handler guarantees the stop runs on Ctrl-C/panic.

**Helpers (exported to downstream slices):** authenticated `*http.Client` (sends `Authorization: Bearer`), run-scoped tag generator (`runID`), per-test cleanup registration, and a suite-teardown sweep that deletes anything tagged with the `runID`.

**Gating:** add a Makefile target `test-integration` → `go test -tags=integration ./test/integration/...`. `make test` / `go test ./...` must not compile or run these tests.

**Tracer test** — `Test_Integration_TracerBullet`: `GET /api/machines/{id}/containers` on the registered machine returns `200` with an empty containers list. This single assertion exercises the entire pipeline.

## Acceptance criteria

- [ ] `backend/test/integration/` exists with `//go:build integration` on all files
- [ ] `go test ./...` (no tag) does **not** compile/run these tests; `make test` is unaffected
- [ ] `make test-integration` boots the podman machine, brings the backend up in-process, and runs the suite
- [ ] `TestMain` starts **and** stops the podman machine; the stop runs under SIGINT/SIGTERM
- [ ] target discovered from `podman system connection list`; every env override honored
- [ ] tracer test passes (`200`, empty containers), proving the full pipeline
- [ ] readiness probe prevents the first test from racing a cold SSH/podman socket
- [ ] run-scoped tag generator, authenticated client, and cleanup helpers are available to downstream slices
- [ ] suite skips cleanly (`t.Skip`) if the `podman` binary or the machine is unavailable

## Blocked by

- 080-00-extract-app (needs `app.New` / `Handler` / `Close`)

## Technical notes

- Temp JWT keys: reuse `internal/testutils.CreateTestJWTKeys`, writing the PEMs to temp files and passing their paths via `*config.Config`.
- `connection_factory.go` writes the decrypted SSH identity to a temp file **without `chmod 0600`** — SSH may reject it. If the tracer fails at the SSH step, treat that as a real finding and surface it (do not silently work around).
- The token comes from the login JSON body and is sent as `Authorization: Bearer` (matches `middleware/auth.go`). Do **not** send it as a cookie.
