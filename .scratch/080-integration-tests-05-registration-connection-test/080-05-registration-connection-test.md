---
title: "Gap #1: connection test at machine registration"
parent: 080-integration-tests
date: 2026-08-11
status: ready-for-agent
labels:
  - ready-for-agent
  - bug
  - backend
  - machines
  - podman-integration
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

Make `POST /api/machines` perform the **documented synchronous connection test** before persisting the machine. Today `MachineService.Create` only validates fields, checks name uniqueness, encrypts the key, and saves — it never opens an SSH/podman connection, so unreachable or mis-credentialed machines register successfully (`201`) and fail later at first use.

Implement the test: attempt an SSH connection to the podman socket using the provided credentials, bounded by `MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS` (default 10). On failure, return `400` with error code `CONNECTION_FAILED` and do **not** persist the machine. Keep it synchronous (registration is documented as synchronous). This is a prod-code behavior fix, not test authoring.

## Acceptance criteria

- [ ] `MachineService.Create` opens a test SSH/podman connection before saving the machine
- [ ] bad host / bad credentials / wrong socket path → `400 CONNECTION_FAILED`; machine **not** persisted
- [ ] valid machine still registers (`201`) and is persisted
- [ ] test is bounded by `MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS`
- [ ] unit tests (mocked) cover the new failure branch
- [ ] the tripwire in 080-04 is updated to expect `400` (bad-host registration no longer returns `201`)

## Blocked by

None - independent prod fix. Coordinate with 080-04 (its tripwire must be flipped when this lands).

## Technical notes

- `description_backend.md` §Machine Registration already specifies this exact flow (timeout `MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS=10`, error code `CONNECTION_FAILED`) — the code simply never implemented it. This issue closes that doc/code gap.
- Reuse the existing connection layer (`connections.ConnectionManager` / `ConnectionFactory`) to perform the probe, so the test path matches the path used at runtime.
