---
title: Machine lifecycle module - owner rules behind one interface, deletion with full teardown
parent: none
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - epic
  - backend
  - machines
  - refactoring
  - testing
author: vibe-agent
---

## What to build

Invert the machines domain: today the HTTP handler owns the Owner rules
(field-merge, PEM validation, key encryption, owner/dup checks, repo writes)
while the service is a row of pass-throughs — and machine deletion never
tears down the connection cache or the SSH-exec pool, despite the domain
model promising it. After this epic there is one deep machine lifecycle
module owning registration, update, sharing, and full graceful teardown; the
handler shrinks to envelope closures like every other domain.

Origin: architecture review 2026-09-09 candidate C2, refined in a grilling
session. The decisions below are settled — slices implement them, they do
not reopen them.

## Design decisions (settled in grilling)

1. **Service boundary**: Update and the sharing rules (list/grant/revoke
   access) move into the machine service. The handler keeps only decoding,
   the endpoint envelope, and DTO mapping. No per-domain service mock is
   introduced; handler tests keep driving the real service over mock repos
   plus the connection-manager mock (the machines-domain exception to the
   engine-seam shape, per backend-conventions.md).
2. **Update is owner-only**: previously any access row — including a
   read-level grantee — could rewrite the owner's hostname, port, and SSH
   private key. Now only the owner may update (403 ACCESS_DENIED otherwise).
   Client-visible contract change; doku hunk required.
3. **Wire contract otherwise byte-identical**: same request/response shapes
   and error codes; `CreateMachineRequest` reuse with empty-means-keep
   semantics stays.
4. **Access checks stop swallowing DB errors**: a broken database answers
   500 INTERNAL_ERROR, never 403 ACCESS_DENIED.
5. **Pass-throughs delete**: the name-uniqueness, duplicate-list, and
   list-owned service methods have zero production callers and fail the
   deletion test.
6. **Delete owns full teardown, DB-first**: DB row → connection-cache
   `RemoveMachine` → SSH-exec pool invalidation, and only after a successful
   DB delete (a failed delete must not evict a live connection). Documented
   race window: an acquisition between row-delete and eviction fails the
   machine lookup (502 CONNECTION_FAILED); in-flight calls finish
   gracefully.
7. **sshexec seam stays narrow**: machines holds the SSH pool through a
   consumer-side interface with one `RemoveMachine(machineID string)` method,
   satisfied by a new public method on the SSH-exec service. The shared
   SSH-exec interface is NOT widened; quadlets and its mock are untouched.
8. **sshexec removal is graceful**, honoring CONTEXT.md §4 ("in-flight calls
   finish first"): pool entries gain refcount + dead marker, mirroring the
   connection cache. Fail-fast invalidation after a failed run and LRU
   eviction's documented posture stay unchanged (non-goals).
9. **Access rows ride the schema**: `machine_access` cascades on machine
   deletion (`ON DELETE CASCADE`, foreign keys on). No explicit cleanup, no
   wider repo interface.
10. **Probe parity is its own slice**: the doku currently claims an
    update-time connection test that the code never runs. The false claim is
    removed by the update slice; a later slice implements key-change probing
    and restores the claim as true.

## Vertical Slices

### 1. Update extraction + owner-only rule
**Issue**: 300-1-update-service-extraction
- Update rules move into the service; PUT becomes owner-only
- doku: owner-only auth note, false probe note removed
- Service rule tests + handler decode tests

**Status**: ready-for-agent
**Blocked by**: None (can start immediately)

### 2. Sharing rules extraction + access-check error fix
**Issue**: 300-2-sharing-rules-extraction
- List/grant/revoke access rules move into the service
- DB errors surface as 500 instead of 403

**Status**: ready-for-agent
**Blocked by**: 300-1 (same files; land sequentially)

### 3. Pass-through deletion
**Issue**: 300-3-passthrough-deletion
- Three zero-caller service pass-throughs delete; List rides the real method

**Status**: ready-for-agent
**Blocked by**: 300-2

### 4. Graceful removal in the SSH-exec pool
**Issue**: 300-4-sshexec-graceful-removal
- Refcount + dead marker on pool entries; public `RemoveMachine`
- Fail-fast invalidate and LRU eviction unchanged

**Status**: ready-for-agent
**Blocked by**: None (parallel to 1-3)

### 5. Full teardown wiring in machine deletion
**Issue**: 300-5-delete-teardown
- Delete = DB row → connection-cache eviction → SSH-exec invalidation
- Narrow `sshInvalidator` seam in machines; mock records removals
- Conventions hunk; 020-2/020-3 slice bookkeeping

**Status**: ready-for-agent
**Blocked by**: 300-4 (and lands after 300-3)

### 6. Probe parity on machine update
**Issue**: 300-6-update-probe-parity
- A new SSH private key triggers the registration-style probe before persisting
- doku note restored as a true statement

**Status**: ready-for-agent
**Blocked by**: 300-1
