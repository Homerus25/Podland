---
title: Prefactor — extract internal/app.App from main.go
parent: 080-integration-tests
date: 2026-08-11
status: ready-for-agent
labels:
  - ready-for-agent
  - testing
  - backend
  - integration-tests
  - refactor
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

Extract an in-process seam from `cmd/server/main.go` so the backend can be brought up under `httptest.NewServer`. This is pure prefactoring — **"make the change easy, then make the easy change"** — with no behavior change. It unblocks the integration harness (080-01).

Introduce a new `internal/app` package with an `App` type that owns every resource and background goroutine currently constructed inline in `main()`, and expose:

- `app.New(cfg *config.Config) (*App, error)` — builds the DB + runs migrations, encryption, auth, repos, connection factory/cache/manager (and starts its `Run` goroutine), sshexec service (and starts its cleanup goroutine), services, handlers, middleware, and the chi router — identical wiring to today's `main()`.
- `(*App) Handler() http.Handler` — returns the configured router.
- `(*App) Close() error` — stops the background goroutines (connection manager, sshexec dialer cleanup) and closes the DB. Safe to call once.

`main()` then shrinks to: load config → `app.New(cfg)` → serve a `*http.Server{Handler: app.Handler()}` on `cfg.Port` → on SIGINT/SIGTERM, `server.Shutdown(ctx)` then `app.Close()`.

**Ownership boundary:** `App` owns resources + goroutines; the caller owns the `*http.Server`. That keeps prod and the in-process test on identical wiring.

## Acceptance criteria

- [ ] `internal/app` package exists with `App`, `New(*config.Config)`, `Handler()`, `Close()`
- [ ] `New` starts the connection manager `Run` goroutine and the sshexec dialer-cleanup goroutine exactly as `main()` does today
- [ ] `Close` stops those goroutines and closes the DB; safe to call once
- [ ] `main()` is reduced to config load + `New` + serve + signal-driven `Shutdown`/`Close`
- [ ] `make build` and `make run` behave identically to before (same routes, same startup logs, same graceful shutdown)
- [ ] `make test` (existing mocked unit tests) still passes

## Blocked by

None - can start immediately.

## Technical notes

- Mirror the exact construction order currently in `cmd/server/main.go` so startup semantics (including the `go connectionManager.Run()` and the sshexec dialer cache) are preserved.
- Keep `config.Load()` in `main()`; `New` takes an already-built `*config.Config` so tests can inject a fully-controlled config (temp DB path, temp JWT PEM paths, random `ENCRYPTION_KEY`).
