---
title: Delete the dead repository twin; one answer to "where do machines persist"
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

The machines domain stops answering "where do machines persist" twice. The
never-constructed concrete repository in the domain package (218 lines of
INSERT/SELECT with zero construction sites) is deleted along with the dead
`GetByNameAndOwner` method on the live models repository (zero call sites
repo-wide). The machine repository interface itself is alive and stays
verbatim — it moves into the domain's seams file next to the encryption seam
and `sshInvalidator`, making that file the single catalogue of consumer seams.
From this slice on there is exactly one machine persistence implementation,
and a compile-time check proves it satisfies the domain's seam instead of the
wiring relying on silence.

For every caller and every test, nothing changes: same interface width, same
wiring in app.go, same green suite.

## Acceptance criteria

- [ ] `backend/internal/domain/machines/repository.go` is deleted; no reference to its concrete struct or constructor remains anywhere in the repo
- [ ] The machine repository interface survives with identical name and identical seven methods in the domain's seams file (`service_interfaces.go`)
- [ ] `models.MachineRepository.GetByNameAndOwner` is deleted; a repo-wide search finds no caller
- [ ] A compile-time assertion in the machines package pins that the production `models.MachineRepository` satisfies the domain's repository interface
- [ ] app.go wiring is untouched: the models repository is still constructed there and handed to the machines service
- [ ] No test behavior changes; `go vet ./...` and `go test ./...` green in the backend module

## Blocked by

- None (can start immediately)
