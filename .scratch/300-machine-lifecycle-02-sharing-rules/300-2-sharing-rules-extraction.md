---
title: Sharing rules extraction + access-check error fix
parent: machine lifecycle module - owner rules behind one interface, deletion with full teardown
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - machines
  - refactoring
  - testing
author: vibe-agent
---

## Parent

Machine lifecycle epic (300-0-machine-lifecycle-master) - read its "Design
decisions" section first; decisions 1 and 4 govern this slice.

## What to build

The rules behind the machine-access endpoints move out of the HTTP handler
closures into the machine service: permission validation, owner-only grant
and revoke, duplicate-grant rejection, cannot-revoke-owner, and access
listing with the caller re-check. The handlers shrink to decode + endpoint
envelope + response mapping. From the user's perspective nothing changes on
the wire except one correctness fix:

The access check stops swallowing database errors. Today a broken database
answers 403 ACCESS_DENIED (the check treats a DB failure as "no access");
after this slice it answers 500 INTERNAL_ERROR. This affects every read
gated on the access check (machine inspect, status, access listing), not
just the sharing endpoints.

## Acceptance criteria

- [ ] Grant rules live in the service: invalid permission 400 INVALID_PERMISSION, unknown machine 404 MACHINE_NOT_FOUND, non-owner grantor 403 ACCESS_DENIED, duplicate grant 400 ACCESS_ALREADY_GRANTED
- [ ] Revoke rules live in the service: non-owner revoker 403 ACCESS_DENIED, self-revoke 400 CANNOT_REVOKE_OWNER, successful revoke 204
- [ ] Access listing lives in the service: caller's access re-checked, 200 with the access list
- [ ] Handlers only decode / read URL params, cross the envelope, and map responses
- [ ] The access check propagates database errors: a failing DB answers 500 INTERNAL_ERROR on gated reads, never 403 ACCESS_DENIED
- [ ] Wire contract otherwise unchanged
- [ ] Service-level tests over mock repos cover every rule above plus the DB-error propagation path
- [ ] Handler tests cover only what the handler owns: malformed JSON body, invalid permission value, missing userId URL param
- [ ] doku unchanged (owner-only notes already exist; the 500 edge falls under the documented generic INTERNAL_ERROR)

## Blocked by

- 300-1-update-service-extraction (same files; land sequentially)
