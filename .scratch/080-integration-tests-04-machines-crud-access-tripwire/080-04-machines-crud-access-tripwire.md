---
title: Machines CRUD + access + gap-#1 tripwire
parent: 080-integration-tests
date: 2026-08-11
status: ready-for-agent
labels:
  - ready-for-agent
  - testing
  - backend
  - integration-tests
  - podman-integration
  - machines
author: vibe-agent
---

## Parent

080-integration-tests — Integration test suite (real podman machine + in-process backend)

## What to build

Integration tests for machine CRUD and access control that exercise those flows **without disturbing the shared good machine** registered during harness setup. Register a **second, throwaway** machine pointing at an unreachable target (e.g. `127.0.0.1:1` with a bogus key). Use it to exercise `PUT`, access grant/revoke/list, and `DELETE` — and as a **tripwire** for gap #1 (issue 080-05).

**Tripwire (clearly commented as gap-#1 behavior):** with the current code, registering the bad-host machine returns `201` — `MachineService.Create` performs **no** connection test today, so the failure surfaces only at the first podman-touching call. Pin this behavior so the test documents reality and flips naturally when 080-05 lands.

## Acceptance criteria

- [ ] registering a bad-host machine currently returns `201` (tripwire, commented as gap-#1 behavior)
- [ ] the bad-host machine's first podman-touching call fails (documents where the failure surfaces today)
- [ ] `PUT /api/machines/{id}` updates name/description/hostname; `GET` reflects the changes
- [ ] `POST .../access` grants read/write/admin; `GET .../access` lists grantees; `DELETE .../access/{userId}` revokes; owner self-revoke is rejected
- [ ] `DELETE /api/machines/{id}` removes the throwaway machine; the shared good machine is untouched (subsequent slices still pass)
- [ ] tripwire test is clearly marked; passes under `make test-integration`

## Blocked by

- 080-01-harness-tracer-bullet (needs the harness and authenticated client)

## Technical notes

- When 080-05 (connection test at registration) lands, **update this slice's tripwire**: bad-host registration should then return `400 CONNECTION_FAILED`, and the "failure at first podman call" assertion is removed. Coordinate with 080-05.
- Keep all throwaway machines named with the run-scoped prefix so the suite sweep can clean them up if a test aborts.
