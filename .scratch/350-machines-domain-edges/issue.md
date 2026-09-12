---
title: Deepen the machines domain's edges (architecture review C3)
parent: none
date: 2026-09-12
status: open
labels:
  - needs-triage
  - backend
  - machines
  - refactoring
author: vibe-agent
---

## Problem Statement

The machines domain — the hottest backend area of the last month — answers
"where do machines persist" twice: `backend/internal/domain/machines/repository.go`
declares a full concrete repository (218 lines of INSERT/SELECT) that is never
constructed anywhere, while the live `models.MachineRepository` is what app.go
actually wires. The domain also declares seams wider than any caller: the
repository interface carries a method the service never calls, and the
encryption interface declares four methods of which exactly one
(`EncryptSSHKey`) is ever invoked. The connection-probe ritual — probe the
plaintext key, map deadline vs. other failure to the two 400 sentinels, then
encrypt and assign — is copy-pasted across Create and Update. And
`internal/encryption` (131 lines, AES-256-GCM for machine SSH keys) has zero
tests: today only mocks ever cross it.

Two corrections to the review, settled during the design session: the
repository interface has **one** unused method (`ListByOwner`), not two; and
`Decrypt`/`DecryptSSHKey` are alive in production — connections and sshexec
depend on the real encryption service directly. Only the machines domain's
*copy* of the encryption seam is over-wide; trimming it orphans nothing.
Additionally, `models.MachineRepository.GetByNameAndOwner` has zero call sites
anywhere and dies with the twin.

## Solution

Delete the dead twin, rehome the repository interface into the domain's seams
file, trim every seam to caller width, own the probe ritual in one helper, and
pin the encryption module with real tests. No endpoint contract changes — the
work is internal to the backend, so no `doku/` updates. Frontend impact: none.

## Decisions (from the design session — do not relitigate)

1. **Interface home**: `repository.go` is deleted entirely; the machine
   repository interface moves verbatim into the domain's seams file
   (`service_interfaces.go`), making it the single catalogue of consumer seams
   (repository, encryption, `sshInvalidator`). The persistence module stays in
   `models` — interface in the consumer, implementation in the provider.
2. **Deletions**: the dead concrete twin; the interface method `ListByOwner`
   (production callers: none — the service never calls it, only mock
   self-tests exercise it); and `models.GetByNameAndOwner` (zero call sites
   repo-wide). Mock-only tests of dead behavior are deleted with their method.
3. **Encryption seam**: shrinks to a single unexported one-method interface
   (`sshKeyEncryptor` with `EncryptSSHKey`), mirroring the unexported
   `sshInvalidator` pattern. The shared `testutils` mock trims to the same
   width, including its error-injecting constructor (one SSH-key error field).
4. **Probe ritual**: one private helper owns the whole invariant — probe the
   plaintext key against the machine's final (defaulted/merged) connection
   fields, map a deadline to `ErrConnectionTestTimeout` and anything else to
   `ErrConnectionTestFailed`, then encrypt and assign ciphertext + fingerprint
   on the machine. Create builds the machine model first; Update keeps its
   key-present guard at the call site (the two consecutive `if` blocks
   collapse into one). No behavior change: same sentinels, same skip-probe
   semantics for updates without a key change, nothing persisted on probe
   failure.
5. **Encryption tests**: the full exported surface of `internal/encryption`
   gets pinned in its own test file — roundtrips, fingerprint determinism,
   tamper/wrong-key rejection, key normalization at construction, and
   `GenerateKey`/`ValidateKey`. The service takes an explicit base64 key, so
   tests run on fixed keys with no environment dependency.

## Slices

- `350-machines-domain-edges-01-delete-dead-twin` — delete the never-constructed
  twin and the dead `GetByNameAndOwner`; rehome the repository interface into
  the seams file; pin the production repo against the seam.
- `350-machines-domain-edges-02-trim-seams-to-caller-width` — repository
  interface drops `ListByOwner` (and its mock-only tests); encryption seam
  becomes the one-method unexported `sshKeyEncryptor`; mock trimmed to match.
- `350-machines-domain-edges-03-verify-key-helper` — the probe + encrypt +
  assign ritual is owned once; Create/Update collapse onto it.
- `350-machines-domain-edges-04-encryption-test-pin` — `internal/encryption`
  gets its first real tests: the interface becomes a test surface.
