---
title: Quadlet Lifecycle endpoints (Start/Stop/Restart)
parent: 070-quadlets-management
date: 2026-08-02
status: ready-for-agent
labels:
  - quadlets
  - backend
  - feature
  - podman-integration
  - ssh
author: vibe-agent
---

## Parent

070-quadlets-management — Implement Quadlet management endpoints (backend) against the podman service API

## Mechanism decision (resolved 2026-08-02)

**Chosen: `systemctl` over SSH.** These endpoints operate on the **systemd unit** a quadlet produces, so they require true systemd semantics (restart policy, auto-update, dependencies). The podman service API exposes no such operation, so lifecycle is implemented by running `systemctl start/stop/restart <unit>` over SSH via the SSH-exec infrastructure (issue 070-10).

## What to build

Implement `POST /quadlets/{quadletId}/start`, `/stop`, `/restart` end-to-end. Each runs `systemctl` against the quadlet's systemd unit over SSH (via `SSHExecInterface` from 070-10):

- Resolve `quadletId` (name) → systemd unit name (the quadlet's `UnitName`, or derive `<name>.service` from the resource suffix)
- Verify the quadlet exists (`QUADLET_NOT_FOUND` / 404 if not) via a podman List lookup
- Start/Stop/Restart the unit: `systemctl start|stop|restart <unit>`, honouring `force` (stop/restart → `systemctl --force`) and `timeout` (stop/restart → `--kill-seconds`)

Each returns `{id, state, unit}` (restart also `restartedAt`). State transitions:

- Start a stopped resource → `state: running`; `QUADLET_ALREADY_RUNNING` (400) if already running
- Stop a running resource → `state: stopped`; supports `force` + `timeout` query params; `QUADLET_NOT_RUNNING` (400) if not running
- Restart → `state: running`; supports `force` + `timeout`; `QUADLET_NOT_RUNNING` (400) if not running

`QUADLET_NOT_FOUND` (404) across all three.

## Acceptance criteria

- [ ] `Service.Start/Stop/Restart` resolve the unit name and run the corresponding `systemctl` command via `SSHExecInterface`
- [ ] `Handler.Start/Stop/Restart` parse `quadletId` + relevant query params and return `200`
- [ ] `QUADLET_ALREADY_RUNNING` / `QUADLET_NOT_RUNNING` / `QUADLET_NOT_FOUND` mapped correctly
- [ ] `force`/`timeout` query params honoured (stop/restart)
- [ ] `CONNECTION_FAILED` on connection errors
- [ ] Service + handler unit tests for each op (success, wrong-state, not-found)

## Blocked by

- 070-02-quadlet-get (quadlet existence + unit-name resolution)
- 070-10-ssh-exec-infrastructure (provides `SSHExecInterface`)

## Technical notes

- No podman service-API primitive exists for systemd-unit lifecycle; this slice depends on 070-10
- Unit-name resolution: prefer the quadlet's `UnitName` (from `QuadletList`); fall back to deriving `<name>.<suffix>.service` from the resource type
- Interpret non-zero `systemctl` exit codes: e.g. unit-already-active → `QUADLET_ALREADY_RUNNING`, unit-not-loaded → `QUADLET_NOT_FOUND`, via `ExecResult.ExitCode`/stderr
- Contract: `doku/description_api.md:988-1069`
