---
title: The key-verification ritual is owned once
parent: 350-machines-domain-edges
date: 2026-09-12
status: open
labels:
  - ready-for-agent
  - backend
  - machines
  - refactoring
author: vibe-agent
---

## Parent

`.scratch/350-machines-domain-edges/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

"A new SSH key earns its place on the machine row" becomes one invariant with
one owner. A single private helper on the machines service takes the machine
and the plaintext key, probes the connection with the machine's final
(defaulted or merged) connection fields, maps a deadline to
`ErrConnectionTestTimeout` and any other probe failure to
`ErrConnectionTestFailed`, then encrypts the key and assigns ciphertext and
fingerprint on the machine. The timeout-mapping comment is written once, in
the helper.

Create builds the machine model before verification (defaults are still
applied before the probe, so the probe dials the real target) and calls the
helper unconditionally — a registration always carries a key. Update keeps
its key-present guard at the call site and calls the helper inside it,
collapsing today's two consecutive `if req.SSHPrivateKey != ""` blocks into
one. Updates without a key change still skip probe and encryption entirely.

Nothing observable changes: same error sentinels in the same situations, no
DB write before a passed probe, identical responses.

## Acceptance criteria

- [ ] Exactly one connection-probe call site in the machines package (the helper); the deadline/other-failure mapping and its comment exist once
- [ ] The helper probes with the machine's connection fields and the plaintext key, then encrypts and sets the encrypted key and fingerprint on the machine
- [ ] Create applies defaults before probing (unchanged), builds the machine model first, then verifies
- [ ] Update has a single key-present guard; a probe never runs for an update without a new key
- [ ] Error mapping unchanged: probe deadline → `ErrConnectionTestTimeout`, other probe failure → `ErrConnectionTestFailed`, encryption failure → the wrapped 500-path error; a failed probe leaves nothing persisted (the existing no-write assertions hold)
- [ ] All existing create/update/connection tests pass unchanged, including mock `TestConnection` expectations and the update no-write test
- [ ] `go vet ./...` and `go test ./...` green in the backend module

## Blocked by

- `350-machines-domain-edges-02-trim-seams-to-caller-width` (the helper's only encryption call is written once against the already-narrowed seam; both slices rewrite the same two service methods)

## Technical notes

- The design session sketched the shape `verifyKey(ctx, m *models.Machine, plaintextKey string) error`; naming is the slice's to refine, the invariant is not.
