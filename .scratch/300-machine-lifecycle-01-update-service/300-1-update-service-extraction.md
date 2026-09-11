---
title: Update extraction + owner-only rule
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
decisions" section first; decisions 1-3 and 10 govern this slice.

## What to build

The rules behind `PUT /api/machines/{id}` move out of the HTTP handler
closure into the machine service: partial field merge (empty-means-keep),
SSH-key PEM validation, key encryption, and the persistence write. The
handler shrinks to decode + endpoint envelope + response mapping, the same
shape as every other domain.

The slice also settles the Update permission rule: only the owner may
update a machine. Today any user with an access row — including a
read-level grantee — can rewrite the owner's hostname, port, and SSH
private key; the doku never promised that. Non-owners receive 403
ACCESS_DENIED, consistent with delete/grant/revoke.

Everything else about the wire contract stays byte-identical: same request
body, same response, same error codes. In particular the doku's stale note
claiming the backend tests the new connection on update is removed (the
probe itself is deliberately out of scope here; slice 300-6 implements it
and restores the claim as true).

## Acceptance criteria

- [ ] Update logic (merge, PEM validation, encryption, write) lives in the machine service; the handler only decodes, crosses the envelope, and maps the response
- [ ] The owner can update; any non-owner with an access row receives 403 ACCESS_DENIED
- [ ] Read- and write-level grantees can no longer change machine fields or credentials
- [ ] Unknown machine still answers 404 MACHINE_NOT_FOUND; invalid PEM still answers 400 INVALID_SSH_KEY
- [ ] Wire contract otherwise unchanged: same body, same success response, same error codes
- [ ] Service-level tests over mock repos cover: owner success, non-owner denial, unknown machine, invalid PEM, partial-field merge (untouched fields preserved), new-key encryption path
- [ ] Handler tests cover only what the handler owns: malformed JSON body
- [ ] doku machines API: PUT section states owner-only authentication; the false connection-test note is removed; no other doku change

## Blocked by

- None (can start immediately)
