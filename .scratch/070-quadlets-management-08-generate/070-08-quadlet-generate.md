---
title: Quadlet Generate endpoint
parent: 070-quadlets-management
date: 2026-08-02
status: ready-for-agent
labels:
  - quadlets
  - backend
  - feature
  - podman-integration
author: vibe-agent
---

## Parent

070-quadlets-management — Implement Quadlet management endpoints (backend) against the podman service API

## What to build

Implement `POST /api/machines/{id}/quadlets/{quadletId}/generate` end-to-end. Generates a quadlet file from an existing container or pod on the remote machine and returns the generated content.

Podman's tunnel does **not** expose a remote `QuadletGenerate`. The conversion logic lives locally in `~/repos/podman/pkg/systemd/quadlet/quadlet.go` (e.g. container-spec → quadlet `.container` unit). Therefore the service:

1. Pulls the container/pod inspect data via the podman API (`ContainerEngine`/`PodEngine` inspect)
2. Runs the `pkg/systemd/quadlet` converter **in the backend** to produce the quadlet file content
3. Returns the generated `content` (does **not** auto-install or auto-start)

This fits the contract response shape, which returns `{…, state: "unknown", content: "<generated>"}` — i.e. the generated file is returned for the user to review/save, not started.

Returns `200` with a generated-quadlet DTO including `content`. Map a missing source resource to `CONTAINER_NOT_FOUND` / `POD_NOT_FOUND` (404). Map conversion failure to `QUADLET_GENERATION_FAILED` (500).

## Acceptance criteria

- [ ] `QuadletGenerateRequest` DTO with `resourceId`, `resourceType` (container|pod), validated
- [ ] `Service.Generate` inspects the source resource via podman API and converts using `pkg/systemd/quadlet`
- [ ] `Handler.Generate` returns `200` with the generated quadlet DTO (including `content`, `state: unknown`)
- [ ] `CONTAINER_NOT_FOUND` / `POD_NOT_FOUND` (404) when the source doesn't exist
- [ ] `QUADLET_GENERATION_FAILED` (500) on conversion failure
- [ ] No files are written/installed on the remote machine by this endpoint
- [ ] Service unit test (mock) covering success (container), success (pod), not-found, conversion-failure
- [ ] Handler unit test covering success, validation failure, not-found

## Blocked by

- 070-01-quadlet-list

## Technical notes

- Conversion reference: `~/repos/podman/pkg/systemd/quadlet/quadlet.go` (container→quadlet around line 595; the package parses a quadlet unit file and emits a systemd unit — invert/Reuse the parser to emit a quadlet from a container spec)
- Inspect via existing `tunnel.ContainerEngine` inspect; pods via the pod engine
- Contract: `doku/description_api.md:1113-1147`
