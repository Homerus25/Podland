---
title: Integration test host — podman-in-podman container replaces the podman machine
parent: none
date: 2026-09-13
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

Replace the podman machine as the backend integration-test host with a **privileged, systemd-booted Fedora container** that is a faithful machine stand-in: sshd, system systemd as PID 1, a user systemd manager (linger), and rootless podman under a `core` user exposing `/run/user/1000/podman/podman.sock`. The backend stays untouched — it registers and dials the container exactly like any machine (`ssh://core@127.0.0.1:24422/...` via the bindings, `systemctl` over SSH for quadlets).

Motivation: provisioning a podman machine inside a CI build image is impractical; a container gives the suite a scriptable, CI-ready test host while keeping full machine parity (quadlets included).

### Resolved rulings (from the grilling session)

- **Environments** — Linux dev boxes + a future self-hosted Linux runner. Exotic hosts (e.g. macOS) keep using the existing `PODLAND_TEST_SSH_*` overrides against an externally managed host; the harness no longer manages any machine.
- **Privilege** — privileged rootful outer container, **podman-only as the outer runtime** (`--systemd=always` handles the cgroup/tmpfs wiring). Inner podman is rootless under `core` (uid 1000) for full parity.
- **Full parity, one environment** — sshd + system systemd + user systemd manager (quadlet lifecycle/auto-update tests depend on it) + rootless socket. No split test matrix.
- **Image** — `FROM fedora:<pinned>` with `podman fuse-overlayfs openssh-server systemd` installed; `core` with subuid/subgid ranges; linger pre-created; `podman.socket` enabled globally for users; `containers.conf` forcing `fuse-overlayfs` (overlay-on-overlay is kernel-denied). `quay.io/podman/stable` was rejected as base: it ships neither systemd nor sshd, and its rolling `stable` tag is less reproducible than a pinned Fedora.
- **Provenance** — Dockerfile in-repo under the integration test tree; built on demand by a Makefile prerequisite of `test-integration`, layer-cached. No registry publishing for now.
- **Runtime shape** — fixed container name, fixed published port `127.0.0.1:24422:22`; flags `--privileged --systemd=always --security-opt label=disable --device /dev/fuse`; two persistent volumes: image cache at `/home/core/.local/share/containers` (rootless store), SSH host keys at `/etc/ssh` (stable key → the bindings' known_hosts auto-append happens exactly once; a changed key is a hard error otherwise).
- **Lifecycle** — `TestMain`-managed per-run container, signal-aware teardown (`podman rm -f`, connection removed, volumes deliberately retained). **Serialized runs per host** — shared store volumes would fight over podman storage locks.
- **SSH identity** — per-run generated ed25519 keypair in the run's temp dir, injected via `podman exec`. Never persisted, never committed (same hygiene as the per-run JWT keys).
- **Connection registration** — harness registers/removes a real `podman system connection` under the renamed `PODLAND_TEST_CONNECTION_NAME` (was `PODLAND_TEST_MACHINE_NAME`), so the `podman --connection` test helpers keep working unchanged. Machine start/stop and `podman system connection list` discovery are deleted.
- **Backend untouched** — the container is purely a harness concern; the machine is registered through the same public API as any other.
- **CI parked** — no CI is built in this epic; the constraint is that everything stays scriptable (CLI-only flags, no interactive prompts, no human-`HOME` assumptions beyond the known_hosts auto-append). A future runner needs Go + podman CLI.
- **Accepted risks** — fuse-overlayfs I/O cost (mitigated by the cache volume; `vfs` is the sanctioned fallback); inner podman may run cgroup-less if delegation is absent (nothing asserts on it); user-manager-in-container is expected-but-unproven until the tracer runs (isolated in 390-02).

## Vertical Slices

### 0. Test-host image + make build target
**Issue**: 390-00-testhost-image
- Pinned-Fedora Dockerfile with the full parity stack (systemd, sshd, `core`, linger, user-global `podman.socket`, fuse-overlayfs) + layer-cached build target wired as `test-integration` prerequisite; manual smoke-verified.

**Status**: ready-for-agent
**Blocked by**: None

### 1. Harness swap — container replaces the machine (tracer)
**Issue**: 390-01-harness-swap
- `TestMain` rewritten: container lifecycle + keypair injection + connection registration (renamed env var) + readiness + signal-aware teardown; machine path deleted; tracer test green against the container; backend untouched.

**Status**: ready-for-agent
**Blocked by**: 390-00

### 2. Full suite green on the container host
**Issue**: 390-02-full-suite-green
- `make test-integration` fully green against the container — quadlet/user-systemd parity, fuse-overlayfs pulls, cache-volume effectiveness; deviations recorded as decisions.

**Status**: completed (2026-09-13)
**Blocked by**: 390-01

**Parity decisions (390-02 outcomes)**:
- **`events_logger = "journald"` in the image's `containers.conf`** (replacing the design-session guess `"file"`): Fedora's default journald log driver refuses `podman logs --follow` over a non-journald events backend — exactly the quadlet/container log-streaming suites. Journald works in-container (systemd is PID 1; rootless container-root maps to core's uid, so its journal entries are readable without group membership), and journald is what a podman machine's Fedora defaults pick — this restores parity rather than working around it.
- **fuse-overlayfs proven effective**: every pulling domain ran green across repeated full runs with zero overlay-on-overlay errors (graph driver `overlay` + the forced `mount_program`). The sanctioned `vfs` fallback was not needed.
- **Cgroup mode recorded**: rootless podman inside the container sees cgroup v2 with the systemd cgroup manager and delegated controllers (cpu/memory/pids) — the "may run cgroup-less" risk did not materialize. Nothing in the suite asserts on cgroup mode either way.
- **Cache volume + stable host key proven**: a second consecutive run reports busybox "already present" (API pull of the present image ~1.6s), hits no known_hosts mismatch, and holds wall-clock at ~158s (214 passing subtests). Measured podman-machine baseline via the `PODLAND_TEST_SSH_*` escape hatch on the same host: 160.4s green — no regression; the container host is marginally faster.
- **Suite bug surfaced by the full run** (not a parity deviation): `OwnerFixtureIntactAfterRejections` asserted the owner held the *sole* grant on the shared machine, but the binary-wide read/write level fixtures (slice 160-00, merged without a full-suite run) legitimately hold grants on it. The tripwire was sharpened to its real invariant — owner keeps admin, the no-access user gains nothing.

### 3. Docs and conventions
**Issue**: 390-03-docs-conventions
- Makefile header env docs, `docs/backend-conventions.md` wording, escape-hatch documentation; no stale machine references.

**Status**: ready-for-agent
**Blocked by**: 390-01

## Technical Context

- **Current wiring** (built by the 080 epic): `backend/test/integration/main_test.go` `TestMain` ensures/starts/stops a podman machine, discovers the target via `podman system connection list`, and the backend dials `ssh://<user>@<host>:<port><socket>` through the Go bindings (`internal/connections/dial.go`). Quadlet paths run `systemctl` over SSH (`internal/sshexec`). Test-side CLI ops go through `podman --connection <suite.MachineName>`.
- **Host-key reality**: the bindings verify SSH host keys against `~/.ssh/known_hosts` — auto-append on first contact, hard error on mismatch; only genuine `podman machine` connections skip verification. A stable host key (persisted `/etc/ssh`) plus a fixed port is what keeps re-runs working.
- **Podman-in-podman facts**: rootful nesting needs `--privileged`; rootless podman as a non-root user inside a privileged container is an officially demonstrated pattern; overlay-on-overlay is denied by the kernel, hence fuse-overlayfs (needs `/dev/fuse`) or `vfs`.
- **Outer runtime is podman**: `--systemd=always` (podman-only) mounts `/run` tmpfs and writable cgroup v2 hierarchies automatically; no Docker flag matrix is carried.

## Acceptance criteria

- [ ] A pinned, in-repo image builds via `make` and boots as a systemd container with sshd, `core`'s user manager, and a working rootless podman (390-00)
- [ ] `make test-integration` runs the suite against the container; no machine is started, stopped, or inspected anywhere (390-01)
- [ ] `PODLAND_TEST_CONNECTION_NAME` replaces `PODLAND_TEST_MACHINE_NAME`; `PODLAND_TEST_SSH_*` remains the escape hatch to externally managed hosts (390-01)
- [ ] Teardown is signal-aware; container and connection are removed, volumes retained; serialized-runs violations fail with a clear error (390-01)
- [x] The entire suite — quadlets included — is green on the container host; re-runs skip re-pulling images and never hit a host-key mismatch (390-02)
- [ ] Docs and conventions describe the container test host with no stale machine claims (390-03)

## Blocked by

None — can start immediately with slice 390-00.
