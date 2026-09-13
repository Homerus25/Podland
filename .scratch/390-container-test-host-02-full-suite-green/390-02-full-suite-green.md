---
title: Full suite green on the container host
parent: 390-container-test-host
date: 2026-09-13
status: completed
labels:
  - completed
  - testing
  - backend
  - integration-tests
  - podman-integration
author: vibe-agent
---

## Parent

390-container-test-host — Integration test host: podman-in-podman container replaces the podman machine

## What to build

The acceptance that the container is a *faithful* machine stand-in: the entire `make test-integration` suite passes against it. This slice exists to isolate the known parity risks — the tracer proved the happy path, this proves the edges — and it is expected to loop fixes back into the image or run flags where reality disagrees with the design:

- **Quadlet suites** (CRUD, lifecycle, logs, generate, import, auto-update) exercise `systemctl`/`systemctl --user` over SSH: the user systemd manager inside a privileged container is the least-proven link. Fixes land in the image (linger, unit enabling) or the run contract — not as test waivers.
- **Image pulls** run through fuse-overlayfs on every domain that pulls busybox/alpine; watch for overlay-on-overlay errors (would indicate the `containers.conf` forcing isn't effective for the actual runtime user).
- **Streaming and logs** (NDJSON pull progress, container/quadlet logs) must behave identically over the SSH-tunneled socket.
- **Cache-volume effectiveness**: a second run must not re-pull images, and overall suite duration must not be pathological compared to the podman-machine baseline.

## Acceptance criteria

- [x] `make test-integration` is fully green against the container host — zero environment-induced skips
- [x] Quadlet lifecycle and auto-update suites pass (user systemd manager works in-container; quadlet-generated containers start/stop)
- [x] Image pulls succeed through fuse-overlayfs with no overlay-on-overlay errors on any pulling domain
- [x] A second consecutive run reuses cached images (no busybox/alpine re-pull) and completes without host-key issues
- [x] Any parity deviation discovered (cgroup delegation absent, storage fallback to `vfs`, needed flag additions) is recorded as a decision in the epic — no silent workarounds

## Blocked by

- 390-01-harness-swap (needs the container-based harness running)

## Technical notes

- Known accepted risks from the design session: fuse-overlayfs I/O cost (cache volume mitigates); rootless podman may run cgroup-less if delegation is absent — nothing in the suite asserts on cgroup mode, so that outcome is fine but should be recorded.
- If fuse-overlayfs proves flaky in practice, `vfs` storage is the sanctioned fallback — a deliberate, recorded downgrade, not a workaround.
- Compare suite wall-clock time against the old machine-based runs once green; a large regression would justify revisiting the cache-volume scope rather than shipping slow.
