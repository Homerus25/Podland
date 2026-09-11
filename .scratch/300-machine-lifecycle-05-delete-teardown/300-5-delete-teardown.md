---
title: Full teardown wiring in machine deletion
parent: machine lifecycle module - owner rules behind one interface, deletion with full teardown
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - machines
  - connections
  - sshexec
  - testing
author: vibe-agent
---

## Parent

Machine lifecycle epic (300-0-machine-lifecycle-master) - read its "Design
decisions" section first; decisions 6-9 govern this slice.

## What to build

Machine deletion owns full teardown, wiring machinery that already exists:
after the DB row is gone, the connection cache's RemoveMachine runs
(graceful, refcounted, already tested), then the SSH-exec pool's removal
(slice 300-4). A deleted machine no longer keeps a live podman socket and
SSH client until idle eviction, and a shared user's in-flight calls finish
rather than landing on a machine whose row is gone.

Ordering is DB-first and teardown runs only after a successful DB delete: a
failed delete must not evict a live machine's connection. The race window
is documented on the delete path in one sentence: an acquisition between
row-delete and eviction fails the machine lookup (502 CONNECTION_FAILED);
the reverse race is already handled by the connection cache's
removed-during-acquisition error.

The machines module holds the SSH pool through a narrow consumer-side seam
declared in the machines package:

    type sshInvalidator interface {
        RemoveMachine(machineID string)
    }

satisfied by the SSH-exec service's new public method. The shared SSH-exec
interface is NOT widened; the quadlets dependency and the shared SSH-exec
mock are untouched. Access rows are not cleaned explicitly: they ride the
schema's cascade on machine deletion, and the delete path says so in a
comment.

## Acceptance criteria

- [ ] Delete performs: DB row removal, then connection-cache RemoveMachine, then SSH-exec pool invalidation
- [ ] Teardown runs only on successful DB delete; a failing delete evicts nothing
- [ ] The sshInvalidator seam lives in the machines package; app wiring passes the real SSH-exec service into the machine service
- [ ] The shared SSH-exec interface and its mock are unchanged
- [ ] The connection-manager mock's RemoveMachine records calls (no longer a silent no-op)
- [ ] Service tests assert the teardown ordering and the failed-delete-evicts-nothing case, using the recording connection mock and a machines-local fake invalidator
- [ ] The cascade reliance and the race-window semantics are documented as comments on the delete path
- [ ] backend-conventions.md machines ruling updated: subtree handlers are envelope closures across the board, delete wires cache + pool teardown, the connection-manager mock records removals
- [ ] The stale 020-2 and 020-3 slice files note their outcome (access cleanup satisfied by the schema cascade; cache invalidation superseded by this epic)

## Blocked by

- 300-4-sshexec-graceful-removal (provides the public RemoveMachine)
- Lands after 300-3-passthrough-deletion (same files)
