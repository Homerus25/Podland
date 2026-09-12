---
title: "internal/encryption gets pinned: the seam's interface becomes a test surface"
parent: 350-machines-domain-edges
date: 2026-09-12
status: open
labels:
  - ready-for-agent
  - backend
  - machines
  - testing
author: vibe-agent
---

## Parent

`.scratch/350-machines-domain-edges/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The AES-256-GCM module that guards every machine's SSH key stops being tested
only through mocks in other packages: it gets its own test file pinning its
full exported surface. Roundtrips for both pairs (`Encrypt`/`Decrypt`,
`EncryptSSHKey`/`DecryptSSHKey`), fingerprint determinism, tamper and
wrong-key rejection, the constructor's key-normalization branches, and the
key helpers — so a regression in any of them fails in this package, not in a
mock's shadow.

Tests construct the service with fixed base64 keys (the constructor takes the
key explicitly; no environment dependency, no global state).

## Acceptance criteria

- [ ] `Encrypt`/`Decrypt` roundtrip; encrypting distinct plaintexts yields distinct ciphertexts, and encrypting the same plaintext twice yields different ciphertexts (per-message nonce)
- [ ] `EncryptSSHKey`/`DecryptSSHKey` roundtrip returns the original key material; the fingerprint is deterministic for the same key and distinct for different keys
- [ ] A tampered ciphertext (flipped byte) and a ciphertext decrypted with a differently-keyed service both fail with an error, not with plaintext
- [ ] Constructor key normalization pinned as-is: invalid base64 errors; decoded keys under 32 bytes are SHA-256-extended; over-long keys are truncated; a 32-byte key passes through untouched
- [ ] `GenerateKey` produces a key the constructor accepts; `ValidateKey` accept/reject cases pinned per current behavior
- [ ] All tests run on fixed in-test keys; no env vars, no globals
- [ ] `go vet ./...` and `go test ./...` green in the backend module

## Blocked by

- None (can start immediately)

## Technical notes

- The constructor signature is `NewEncryptionService(key string)` with base64-encoded input; generate fixture keys with `base64.StdEncoding` in the test, not by hand.
