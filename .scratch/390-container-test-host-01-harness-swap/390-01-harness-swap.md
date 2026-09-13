---
title: Harness swap — container replaces the machine (tracer)
parent: 390-container-test-host
date: 2026-09-13
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

390-container-test-host — Integration test host: podman-in-podman container replaces the podman machine

## What to build

`TestMain` in `backend/test/integration/` swaps the podman-machine lifecycle for the container test host, and the existing tracer test proves the whole pipeline (HTTP → JWT → machine registration → SSH tunnel → podman socket → HTTP) against the container. Everything machine-specific in the harness is deleted; the backend product code is **untouched** — the container is registered through the same public API as any machine.

`TestMain` performs, in order:

1. **Ensure image.** Fail fast with an actionable message ("run `make <build-target>`") if the test-host image is absent.
2. **Start container.** `podman run --detach` with the run contract from 390-00: `--privileged --systemd=always --security-opt label=disable --device /dev/fuse`, fixed container name, `-p 127.0.0.1:24422:22`, and the two persistent volumes (image cache → `/home/core/.local/share/containers`, SSH host keys → `/etc/ssh`). A name/port collision fails with a clear "another integration run is active on this host" error (serialized-runs model).
3. **Inject identity.** Generate a fresh ed25519 keypair into the run's temp dir; inject the public key as `core`'s `authorized_keys` via `podman exec` (as root: write, chown `core`, correct permissions).
4. **Register connection.** `podman system connection add <name> --identity <run-key> ssh://core@127.0.0.1:24422/run/user/1000/podman/podman.sock`, where `<name>` comes from the renamed `PODLAND_TEST_CONNECTION_NAME`. This keeps every `podman --connection <name>` test helper working unchanged. When `PODLAND_TEST_SSH_*` overrides are set, container lifecycle is skipped entirely and the suite targets that external host (the escape hatch).
5. **Readiness.** Wait for the published sshd port, then retry a real podman-over-SSH probe (`podman --connection <name> ps`) with exponential backoff (~60s budget) — replacing machine-state polling.
6. **Run, then teardown** (reverse order, under the existing SIGINT/SIGTERM handler so Ctrl-C/panic still cleans up): close httptest server → `app.Close()` → remove temp dirs → `podman system connection rm` → `podman rm -f` the container. Volumes are deliberately retained.

**Deletions:** machine ensure/start/stop, `podman machine inspect` polling, and `podman system connection list` discovery all disappear. The clean-skip contract survives: no `podman` binary on the host → skip the suite, exactly like today.

## Acceptance criteria

- [ ] The existing tracer test passes against the container through the rewritten `TestMain`
- [ ] No code path starts, stops, or inspects a podman machine anymore; connection discovery is gone
- [ ] `PODLAND_TEST_CONNECTION_NAME` is honored; `PODLAND_TEST_MACHINE_NAME` no longer exists anywhere
- [ ] Setting `PODLAND_TEST_SSH_*` bypasses the container lifecycle and targets the external host
- [ ] Second run on the same host: no SSH host-key mismatch; `~/.ssh/known_hosts` holds at most one `[127.0.0.1]:24422` entry ever
- [ ] Ctrl-C mid-run: connection removed, container removed, volumes retained
- [ ] Missing image → immediate failure naming the make target; missing `podman` binary → clean skip
- [ ] Name or port already in use → clear serialized-runs error, not a cryptic podman failure
- [ ] No changes outside the integration test tree and the backend Makefile — the product never learns the container exists

## Blocked by

- 390-00-testhost-image (needs the image and its run contract)

## Technical notes

- The Go bindings verify SSH host keys against `~/.ssh/known_hosts` (auto-append on first contact, hard error on mismatch); only genuine `podman machine` connections skip verification. The persistent `/etc/ssh` volume plus the fixed port is what makes the auto-append a one-time event.
- The per-run keypair mirrors the suite's per-run JWT key hygiene: nothing private persists beyond the run's temp dir.
- `suite.MachineName` semantics shift from "podman machine name" to "registered connection name"; sweeps and run-scoped tags are unaffected (they key off `RunID` and API-registered names).
- Keep the run contract (flags, volumes, port, name) in one place in the harness — it is the automation of 390-00's documented smoke command.
