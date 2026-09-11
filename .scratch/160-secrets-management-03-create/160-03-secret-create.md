---
title: Secret Create endpoint
parent: 160-secrets-management
date: 2026-08-27
status: closed
labels:
  - ready-for-agent
  - backend
  - secrets
  - feature
  - podman-integration
author: vibe-agent
---

## Parent

160-secrets-management — Implement Secret management endpoints (backend) against the podman service API

## What to build

Implement `POST /api/machines/{id}/secrets` (write-level route via `RequireMachinePermission(write)` from 160-00). Podman takes the secret value as the raw request body reader and the name in options — `secrets.Create(connCtx, reader, opts)` (`~/repos/podman/pkg/bindings/secrets/secrets.go:71`).

- Request DTO `SecretCreateRequest{Name string, Value string}` with validator tags:
  - `Name`: required, `max=255`, pattern `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` (podman's secret name rules); **trim** the name
  - `Value`: required, `min=1`, `max=1048576` (1 MiB); **never trim or otherwise mutate the value** — leading/trailing whitespace and newlines are significant
- Service: `strings.NewReader(req.Value)` as the body, `new(secrets.CreateOptions).WithName(req.Name)` (defaults: file driver, no labels — deferred by ruling)
- Response: `201` with `{id}` from `entities.SecretCreateReport`
- Map name-conflict failures to 409 `SECRET_ALREADY_EXISTS`, other creation failures to 500 `SECRET_CREATION_FAILED`
- The value must not be logged (validate before logging request bodies if a logging middleware exists for them; quadlets logs content, so audit this path deliberately)

## Acceptance criteria

- [x] `POST` gated at `write`: `read` → 403, `write`/`admin` → 201
- [x] Value reaches podman byte-identical (unit test with value containing leading/trailing spaces and an embedded newline)
- [x] 409 `SECRET_ALREADY_EXISTS` on duplicate name; 400 `VALIDATION_FAILED` on bad name/oversized/empty value
- [x] Service unit tests with mock connection manager (assert options and reader wiring); handler tests for each status
- [x] No log statement in the slice can emit the request value

## Blocked by

- 160-01-secret-list
- 160-00-level-aware-access

## Technical notes

- Bindings precedent for body-reader ops: `quadlets.Install(connCtx, options, body)` (`internal/domain/quadlets/service.go:174`)
- Options reference: `CreateOptions{Name, Driver, DriverOpts, Labels, Replace, Ignore}` (`~/repos/podman/pkg/bindings/secrets/types_create_options.go`) — only `Name` is used in v1
- Return the created ID only, never the value
