---
title: Test-host image + make build target
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

An in-repo Dockerfile (under the backend integration test tree) that produces the **machine-stand-in test host image**, plus a Makefile target that builds it with layer caching and is wired as a prerequisite of `test-integration`. Standalone-verifiable via a manual smoke run: the image must boot as a systemd container in which every capability the suite relies on already works.

The image contains, on a **pinned Fedora base**:

- `podman`, `fuse-overlayfs`, `openssh-server`, `systemd` installed via dnf.
- A `core` user with **uid/gid 1000** and subuid/subgid ranges (rootless podman identity).
- Linger pre-created for `core` (the `/var/lib/systemd/linger/core` marker) so the user systemd manager exists from boot without a login session.
- `podman.socket` enabled **globally for users**, so `core`'s user manager activates `/run/user/1000/podman/podman.sock` at boot.
- sshd enabled; host-key generation happens at first boot (keys land in the `/etc/ssh` volume, nothing baked into the image).
- A rootless `containers.conf` forcing `fuse-overlayfs` as the mount program — overlay-on-overlay is denied by the kernel, so the inner rootless podman must not attempt native overlay on the container's own overlay filesystem.
- **No** committed SSH host keys and **no** committed `authorized_keys`.

The smoke run exercises exactly the contract the harness will rely on: `podman run --detach --privileged --systemd=always --security-opt label=disable --device /dev/fuse` (plus the two persistent volume mounts for `/etc/ssh` and `/home/core/.local/share/containers`) → systemd is PID 1, sshd is reachable, and as `core`: the user manager is running, the podman socket exists, and `podman run busybox` succeeds over fuse-overlayfs.

## Acceptance criteria

- [ ] A `make` target (e.g. `testhost-build`) builds the image with `podman build`; no manual steps required
- [ ] `make test-integration` declares it as a prerequisite; building twice is a no-op thanks to layer caching
- [ ] Manual smoke run per the documented flags: systemd is PID 1 and sshd is active inside the container
- [ ] As `core`: user systemd manager running (linger effective) and `podman.socket` listening at `/run/user/1000/podman/podman.sock`
- [ ] As `core`: `podman run busybox` succeeds using fuse-overlayfs storage — no overlay-on-overlay errors
- [ ] Image contains no SSH host keys and no `authorized_keys`; Fedora base is pinned, not `latest`

## Blocked by

- None (can start immediately)

## Technical notes

- uid **must** be exactly 1000: the suite registers and asserts the rootless socket path `/run/user/1000/podman/podman.sock`.
- `quay.io/podman/stable` was evaluated and rejected as the base: it ships neither systemd nor sshd, and once those are added its only residual value is the ~3-line fuse-overlayfs `containers.conf` that this image replicates anyway.
- User-unit enabling without a running systemd during build: create the `/etc/systemd/user/podman.socket.wants/` symlink (or `systemctl --global enable` equivalent) rather than invoking a live `systemctl --user`.
- The smoke command (flags + volume mounts) is the de-facto run contract that 390-01 automates — keep it written down next to the Makefile target.
