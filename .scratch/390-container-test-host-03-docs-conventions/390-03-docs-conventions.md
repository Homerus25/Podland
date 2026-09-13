---
title: Docs and conventions
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

Make the written record match the new reality after the harness swap, so nobody follows stale machine-era instructions:

- **Makefile header docs** (`backend/Makefile`, above `test-integration`): document `PODLAND_TEST_CONNECTION_NAME` (default, what it names now — the registered podman connection) and re-document `PODLAND_TEST_SSH_{HOST,PORT,USER,SOCKET,KEY_PATH}` as the **escape hatch to an externally managed host** (e.g. a manually started podman machine on macOS), which bypasses the container lifecycle entirely.
- **`docs/backend-conventions.md`**: replace the "run against a real podman machine" wording for the integration suite with the container test host; note the serialized-runs-per-host model (fixed name/port, shared cache volumes) so a second concurrent run's error is understood, not mysterious.
- Sweep the docs reachable from AGENTS.md navigation for any other claim that the integration suite boots/stops a podman machine, and fix it.

## Acceptance criteria

- [ ] Makefile header documents `PODLAND_TEST_CONNECTION_NAME` and the `PODLAND_TEST_SSH_*` escape hatch accurately
- [ ] `docs/backend-conventions.md` describes the container test host and the serialized-runs model; no podman-machine claims remain for the integration suite
- [ ] No stale references to `PODLAND_TEST_MACHINE_NAME` or machine start/stop in the docs tree
- [ ] `doku/` API endpoint docs untouched — the backend contract is unchanged, which is the point

## Blocked by

- 390-01-harness-swap (needs the final env names and behavior to document)

## Technical notes

- The parked CI constraint (scriptable, CLI-only, no interactivity; future runner needs Go + podman CLI) is recorded in the epic master — only reference it here if the conventions doc naturally mentions CI readiness.
