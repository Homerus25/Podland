---
title: Probe parity on machine update
parent: machine lifecycle module - owner rules behind one interface, deletion with full teardown
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - machines
  - connections
  - testing
author: vibe-agent
---

## Parent

Machine lifecycle epic (300-0-machine-lifecycle-master) - read its "Design
decisions" section first; decision 10 governs this slice.

## What to build

An owner updating a machine's SSH private key can no longer silently save
broken credentials: when a new key is provided, the update path runs the
same registration-style connection probe (against the resulting merged
machine configuration) before persisting anything. A bad key, host, or
socket answers 400 CONNECTION_FAILED, and a probe deadline answers 400
CONNECTION_TEST_TIMEOUT — both codes already listed in the doku's PUT error
table, which is why this slice makes the documented contract true instead
of inventing a new one. The doku note removed in slice 300-1 ("If
sshPrivateKey is provided, the backend will test the new connection")
returns as a true statement.

Updates that do not change the key skip the probe; everything else about
the wire contract is unchanged.

## Acceptance criteria

- [ ] An update providing a new SSH private key probes the merged connection configuration before persisting; a failed probe persists nothing and answers 400 CONNECTION_FAILED
- [ ] A probe deadline answers 400 CONNECTION_TEST_TIMEOUT
- [ ] Updates without a key change skip the probe entirely
- [ ] The probe runs before any write, matching the registration flow's nothing-left-behind property
- [ ] doku machines API: the PUT connection-test note is restored truthfully
- [ ] Service tests over the connection-manager mock cover probe success (update persists), probe failure (nothing persists), probe timeout, and probe-skipped (no key change)

## Blocked by

- 300-1-update-service-extraction (the update rules live in the service, where the probe belongs)
