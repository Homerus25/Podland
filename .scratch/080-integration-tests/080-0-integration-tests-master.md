---
title: Integration test suite — real podman machine + in-process backend
parent: none
date: 2026-08-11
status: ready-for-agent
labels:
  - ready-for-agent
  - epic
  - testing
  - backend
  - integration-tests
  - podman-integration
author: vibe-agent
---

## What to build

A Go integration test suite that exercises the **full Podland pipeline** (HTTP → JWT → machine → SSH tunnel → podman socket → HTTP) against a **real podman machine**. Today every test in the repo is mock-based — including the misleadingly-named `internal/domain/containers/integration_test.go`, which uses `MockContainerService`. The existing `.scratch/010` and `.scratch/050` testing issues are explicitly *mock-based* ("deterministic, no external state"). This epic adds true end-to-end tests for the first time.

This epic is **backend-only**, consistent with the 030/060/070 work.

### Resolved rulings (from the grilling session)

- **SUT mode — in-process.** The backend runs under `httptest.NewServer`, not as a subprocess or container. The SSH→podman path is fully real; only the TCP listener is virtual. Requires extracting an `App` seam from `cmd/server/main.go` (slice 080-00).
- **Machine lifecycle — start AND stop per run.** The suite boots the podman machine in `TestMain` and stops it on exit. Teardown is signal-aware (SIGINT/SIGTERM) so Ctrl-C / panic still stops the VM.
- **Target discovery — hybrid.** Parse `podman system connection list` for the connection matching `PODLAND_TEST_MACHINE_NAME` (default `podman-machine-default`) → host/port/ssh-user/in-VM socket path + Identity key path. Each field is overridable via `PODLAND_TEST_SSH_{HOST,PORT,USER,SOCKET,KEY_PATH}` so the same suite can also target the `pi` remote or any host. This survives start/stop SSH-port reallocation.
- **Auth — seed + real login.** Seed one user (bcrypt, mirroring `cmd/initdb`) into the temp DB, then exercise the real `POST /api/auth/login`; capture the token from the JSON body; send as `Authorization: Bearer` on protected calls. (This matches the actual middleware, which reads the Bearer header — **not** the docs' "HTTP-only cookie" claim; that doc drift is gap #3, slice 080-07.)
- **Registration — once, shared.** Register the discovered machine once via `POST /api/machines` as the seeded user; share the returned machine ID across all tests. Reachability is proven by the first podman-touching call, not by registration (gap #1, slice 080-05).
- **Readiness — poll + real probe.** Poll `podman machine inspect` until `State==running`, then retry a real podman-over-SSH probe with exponential backoff (~60s budget). VM "running" ≠ socket ready.
- **Isolation — run-scoped tag + per-test cleanup + suite sweep.** Everything the suite creates carries a run-scoped token (`podland-test-run=<runID>` / names `it-<runID>-…`). Each test registers `t.Cleanup` for exactly what it created. List/count assertions filter by the run tag so a pre-dirty machine doesn't cause false failures. Suite teardown sweeps anything still tagged.
- **Gating — build tag + dedicated dir.** All files live in `backend/test/integration/` behind `//go:build integration`. `make test-integration` runs them; `make test` / `go test ./...` are unaffected and never accidentally boot a VM.

### Scope (first cut)

A vertical slice using **busybox** (tiny, fast pull): machines list/get · containers empty-list + lifecycle · images pull/list/delete. Quadlets (systemctl over SSH), events (SSE), and rate-limiting are **deferred** to later issues once the harness exists.

## Vertical Slices

Each slice cuts through the harness + a domain end-to-end for one concern.

### 0. Prefactor — extract `internal/app.App`
**Issue**: 080-00-extract-app
- New `internal/app` package: `App`, `New(cfg)`, `Handler()`, `Close()`
- `App` owns resources + background goroutines (connection manager, sshexec dialer cleanup)
- `main()` shrinks to config load + `New` + serve + signal-driven `Shutdown`/`Close`
- Prod behavior identical

**Status**: ready-for-agent
**Blocked by**: None

### 1. Integration harness + tracer-bullet test
**Issue**: 080-01-harness-tracer-bullet
- `TestMain`: start/stop podman machine (signal-aware) · discover via `podman system connection list` + env overrides · bring backend up in-process (`app.New` + `httptest.NewServer`) with temp DB / temp JWT PEMs / random `ENCRYPTION_KEY` · seed user · real login → Bearer · register machine once · readiness poll + probe
- Helpers: authenticated client, run-scoped tag, per-test cleanup, suite sweep
- `//go:build integration` + `make test-integration`
- **One test**: `GET /api/machines/{id}/containers` → 200 empty (proves the whole pipeline)

**Status**: ready-for-agent
**Blocked by**: 080-00

### 2. Images vertical slice
**Issue**: 080-02-images-slice
- Pull busybox (consume NDJSON progress stream) · list images (run-tag filtered) · delete image

**Status**: ready-for-agent
**Blocked by**: 080-01

### 3. Containers vertical slice
**Issue**: 080-03-containers-slice
- Ensure busybox (idempotent) · create · start · stop · get · delete — run-scoped names + per-test cleanup

**Status**: ready-for-agent
**Blocked by**: 080-01

### 4. Machines CRUD + access + gap-#1 tripwire
**Issue**: 080-04-machines-crud-access-tripwire
- Register a SECOND throwaway bad-host machine; pin current behavior as a tripwire (201 + failure at first podman call)
- On that throwaway: update · grant/revoke access · list access · delete — without disturbing the shared good machine

**Status**: ready-for-agent
**Blocked by**: 080-01

### 5. Gap #1 — connection test at registration
**Issue**: 080-05-registration-connection-test
- `MachineService.Create` performs the documented synchronous SSH/podman test (`MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS`) → `CONNECTION_FAILED` 400 on bad host
- Flips 080-04's tripwire

**Status**: ready-for-agent
**Blocked by**: None (independent prod fix; coordinate with 080-04)

### 6. Gap #2 — verify password in auth.Login
**Issue**: 080-06-auth-password-verification
- `bcrypt.CompareHashAndPassword` in `POST /api/auth/login`; `INVALID_CREDENTIALS` on mismatch
- Add integration wrong-password → 401 test

**Status**: ready-for-agent
**Blocked by**: None

### 7. Gap #3 — docs: JWT is Bearer, not cookie
**Issue**: 080-07-docs-jwt-bearer
- Fix `CONTEXT.md` §5 and `description_backend.md` to match the implementation

**Status**: ready-for-agent
**Blocked by**: None

## Technical Context

- **Current wiring**: all dependency wiring is inline in `cmd/server/main.go` (config → DB → migrations → encryption → auth → repos → connection manager + goroutine → sshexec + goroutine → services → handlers → middleware → router → `*http.Server`). No reusable seam exists today — hence 080-00.
- **Connection model**: `internal/connections/connection_factory.go` builds `ssh://<user>@<host>:<port><socket>` via `bindings.NewConnectionWithOptions`, writing the decrypted SSH key to a temp file. **Heads-up:** that temp file is never `chmod 0600` — SSH may reject it. The tracer test (080-01) will surface this if it bites; treat it as a real finding rather than working around it silently.
- **Auth reality vs docs**: `POST /api/auth/login` returns the token in the JSON body (`LoginResponse{Token}`) and the middleware reads `Authorization: Bearer`. Login does **not** verify the password today (gap #2 → 080-06).
- **Module constraint**: `go.mod` has a local `replace go.podman.io/podman/v6 => /home/homerus/repos/podman/`, so the backend (and these tests) currently build only on this machine.
- **Established patterns**: mirror `internal/domain/images/` / `containers/` for DTO + service-interface + handler-test conventions in the unit-test layer; the integration suite sits alongside, not instead of, those mocked tests.

## Acceptance criteria

- [ ] `internal/app.App` seam exists; `main()` is thin; prod runs identically (080-00)
- [ ] `make test-integration` boots a real podman machine, runs the backend in-process, and a tracer test passes proving the full pipeline (080-01)
- [ ] `make test` / `go test ./...` are unaffected by the integration suite
- [ ] Image pull/list/delete and container create/start/stop/delete work over the real machine (080-02, 080-03)
- [ ] Machine CRUD + access control tested without disturbing the shared machine; gap-#1 behavior pinned by a tripwire (080-04)
- [ ] Registration performs a real connection test (080-05); login verifies passwords (080-06); docs match the Bearer implementation (080-07)
- [ ] Tests are isolated via run-scoped tagging + per-test cleanup + suite sweep; no leftover junk on the shared machine
- [ ] Teardown is signal-aware; the podman machine is stopped even on Ctrl-C/panic

## Blocked by

None - can start immediately with slice 080-00 (prefactor).
