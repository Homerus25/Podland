---
title: Secrets integration tests + API contract documentation
parent: 160-secrets-management
date: 2026-08-27
status: open
labels:
  - ready-for-agent
  - backend
  - secrets
  - feature
  - test
  - documentation
author: vibe-agent
---

## Parent

160-secrets-management — Implement Secret management endpoints (backend) against the podman service API

## What to build

Close the epic: real-podman integration tests for the four endpoints plus the permission matrix, and the shared API contract documentation.

**Integration tests** — extend the existing harness (`backend/test/integration/`, established by the 080 series) with a secrets slice:

- List/Inspect/Create/Delete against a real podman in the test pod, following the 080-08 (quadlets CRUD) slice structure
- Round-trip: create → appears in list → inspect metadata has no value → delete → gone
- Reveal: create via API, `?reveal=true` as admin returns the exact value (byte-identical, incl. whitespace); reveal as `write` and `read` users → 403
- Permission tripwires (complements 160-00): `read` user → 403 on POST and DELETE; `write` user → 403 on reveal
- Name-conflict → 409; delete in-use secret → 409 (create a container referencing the secret, if the harness supports it — otherwise unit-cover and note the gap)

**Documentation** — `doku/description_secrets_api.md` mirroring `description_quadlets_api.md`:

- Route table, request/response shapes (metadata DTO; `SecretData` only on admin reveal), error codes
- Permission matrix: read → list/inspect(metadata); write → +create/delete; admin → +reveal
- Explicit deviations: `id` accepts name or ID; no update endpoint (immutable); value write-only except admin reveal; 1 MiB value cap
- Cross-link from `doku/description_api.md` next to the other per-feature contracts

## Acceptance criteria

- [ ] Secrets integration suite passes in the podman-based harness alongside the existing slices
- [ ] Round-trip + reveal-exact-value + all 403 tripwires covered
- [ ] `doku/description_secrets_api.md` exists, linked from the contract index, matching implemented behaviour
- [ ] `go test ./...` green in backend; no regressions in the 080 suites

## Blocked by

- 160-01-secret-list
- 160-02-secret-inspect-reveal
- 160-03-secret-create
- 160-04-secret-delete

## Technical notes

- Harness entry points: see `.scratch/080-integration-tests/080-0-integration-tests-master.md` and the quadlets slices 080-08..080-12 for fixtures (machine registration, user seeding with access levels)
- Seed users per level (read/write/admin) the way 080-04's access tripwire does
