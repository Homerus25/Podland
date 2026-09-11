---
title: Graceful removal in the SSH-exec pool
parent: machine lifecycle module - owner rules behind one interface, deletion with full teardown
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - sshexec
  - machines
  - refactoring
  - testing
author: vibe-agent
---

## Parent

Machine lifecycle epic (300-0-machine-lifecycle-master) - read its "Design
decisions" section first; decision 8 governs this slice.

## What to build

The SSH-exec pool (the cached per-machine SSH clients behind systemctl and
auto-update runs) gains a graceful machine-removal operation, honoring the
recorded domain decision that removal is graceful: in-flight calls finish
first. The connection cache already implements this discipline (refcount +
dead marker on entries); this slice mirrors it in the SSH-exec pool.

From the user's perspective: an owner deleting a machine no longer leaves a
live SSH session behind, and a shared user's running systemctl command
completes instead of dying mid-run.

Removal is exposed as a public `RemoveMachine(machineID)` on the SSH-exec
service, mirroring the connection manager's vocabulary, so the machines
module can call it at deletion time (wired in slice 300-5). The existing
fail-fast invalidation (drop the stale client after a failed run) keeps its
current meaning and stays. LRU eviction's documented posture (may close an
in-use client; short-lived execs, fail-fast) is explicitly a non-goal of
this slice.

## Acceptance criteria

- [ ] Removal while an exec is in flight: the exec runs to completion; the SSH client closes when the last in-flight exec releases it
- [ ] Removal of an idle entry: the client closes immediately
- [ ] Removal of an unknown machine: no-op, no error
- [ ] No tombstone: a later exec for a removed machine simply dials again (and fails on the gone machine row — that end-to-end pinning belongs to slice 300-5)
- [ ] Removal is safe to call repeatedly (idempotent)
- [ ] Fail-fast invalidation after a failed run behaves exactly as before
- [ ] LRU eviction behaves exactly as before
- [ ] Pool tests (fake dialer/runner pattern already in the package) pin the graceful, idle, no-op, re-dial, and idempotence paths

## Blocked by

- None (can start immediately; parallel to slices 1-3)
