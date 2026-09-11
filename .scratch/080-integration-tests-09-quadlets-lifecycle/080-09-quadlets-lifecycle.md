---
title: Quadlet Lifecycle vertical slice — start, stop, restart over systemctl/SSH
parent: 080-integration-tests
date: 2026-08-13
status: ready-for-agent
labels:
  - ready-for-agent
  - testing
  - backend
  - integration-tests
  - podman-integration
  - quadlets
  - ssh
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

Integration tests covering the quadlet lifecycle (start / stop / restart) over the real podman machine. These endpoints run `systemctl --user <action> <unit>` over SSH via the `sshexec` service (already wired into `app.New`), so this slice proves the SSH-exec + systemd path end-to-end — the concern the 080 master explicitly deferred.

Create a runnable `.container` quadlet (recipe + `createQuadlet` helper from 080-08), then:

- **Start** → poll `GET /quadlets/{name}` (QuadletList-derived `state`) until `running`; assert `QUADLET_ALREADY_RUNNING` (400) on a second start.
- **Stop** → poll until `stopped`; assert `QUADLET_NOT_RUNNING` (400) on a second stop.
- **Restart** → returns to `running`.

State is verified via GET (the inspect-backed QuadletList `Status` parse), NOT the start/stop response body — the response hardcodes its `state` field and would mask a transition that never actually happened (same reasoning as `waitForContainerState` in the containers slice).

Also hosts the active **DELETE-on-running tripwire** (gap pinned in 080-08): after starting the quadlet, `DELETE` without `force` returns `QUADLET_RESOURCE_RUNNING` (400) — the current no-`stop`-wiring behavior. With `force=true`, `DELETE` succeeds (204). Clearly comment as gap behavior.

## Acceptance criteria

- [ ] start transitions the quadlet to `running` (verified via GET poll, ~30s budget)
- [ ] second start on an already-running quadlet returns `QUADLET_ALREADY_RUNNING` (400)
- [ ] stop transitions the quadlet to `stopped` (verified via GET poll)
- [ ] second stop on a non-running quadlet returns `QUADLET_NOT_RUNNING` (400)
- [ ] restart returns the quadlet to `running`
- [ ] `force` / `timeout` query params accepted (stop/restart)
- [ ] lifecycle on a missing quadlet returns `QUADLET_NOT_FOUND` (404)
- [ ] DELETE on a running quadlet without `force` returns `QUADLET_RESOURCE_RUNNING` (400) — pinned gap (commented)
- [ ] DELETE on a running quadlet with `force=true` returns `204`
- [ ] created quadlet + its spawned container are cleaned up via `t.Cleanup` + the suite sweep
- [ ] test passes under `make test-integration`

## Blocked by

- 080-08-quadlets-crud (needs the `createQuadlet` helper, the runnable-quadlet recipe, and the sweep pattern)

## Technical notes

- The unit name is the quadlet's `UnitName` (from QuadletList), typically `<name>.service` for a `.container` quadlet. The service resolves it via `resolveForLifecycle`; you only need the quadlet **name** for the HTTP path.
- Polling: mirror `waitForContainerState` — a `waitForQuadletState(t, machineID, name, want)` helper polling `GET /quadlets/{name}` every ~500ms, 30s budget. The `state` field is parsed from systemd `Status` by `parseStateFromStatus`, so propagation may lag the `systemctl` return by a moment.
- `systemctl --user` runs as the machine's SSH user; the podman machine must support it (rootless machines do). If the machine can't run user systemd units, `t.Skip` with a clear reason rather than failing.
- Cleanup must stop the unit AND remove the quadlet AND remove the spawned container: `podmanConnRun("quadlet", "rm", "-f", name)` handles quadlet+unit; the spawned container is run-scoped-named and covered by the containers sweep, but also `podmanConnRun("rm", "-f", <container>)` to be safe.

## Resolution notes (2026-08-14)

Implemented in `backend/test/integration/quadlets_lifecycle_test.go`. Two empirical deviations from the AC text, both discovered against the real machine and pinned in the test with comments:

- **Stop does not produce state `stopped`.** A stopped unit reports systemd status `inactive/dead`, which `parseStateFromStatus` (dto.go) maps to `unknown` (a force-stop maps to `error` via `failed`). The stop transition is therefore verified as "state leaves `running`" (GET-poll), which is the observable contract; waiting for a literal `stopped` would hang forever.
- **`timeout` param is broken in the backend, pinned as a gap tripwire.** `LifecycleOptions.Timeout` maps to `systemctl --kill-seconds=<n>` (lifecycle.go `buildLifecycleArgs`), but `--kill-seconds` is not a systemctl option on any systemd version — systemctl exits 1 with "unrecognized option" and the API returns 500 `INTERNAL_ERROR`. The test pins this (`StopWithTimeoutParamFailsGap`); `force` is exercised and accepted on both stop and restart.
- Cleanup note: `podman quadlet rm` is not implemented for remote CLI clients (the released CLI rejects quadlet subcommands over `--connection`), so cleanup force-deletes via the API (which stops a running unit server-side) and `podman rm -f`s the `systemd-<name>` spawned container. Verified no leftovers after runs.
- The missing-quadlet skip probe (`requireUserSystemd`) dials the machine's SSH with the exact coordinates the backend registered, so it probes the real transport `systemctl --user` uses.
