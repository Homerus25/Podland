---
title: Secrets UI — delete with confirmation
parent: 170-secrets-ui
date: 2026-08-27
status: open
labels:
  - ready-for-agent
  - frontend
  - secrets
  - feature
author: vibe-agent
---

## Parent

`.scratch/170-secrets-ui/issue.md`

## What to build

Add per-row secret deletion to the Secrets tab, following the `DeleteQuadletModal` pattern exactly: a row-level delete action (own mutation instance so pending/errors stay row-scoped), a confirmation modal naming the secret, a red confirm button with `data-testid="confirm-delete-button"`, and list invalidation on success.

- `deleteSecret(machineId, secretId)` → `DELETE /machines/${machineId}/secrets/${secretId}` (name or ID as the identifier — the list DTO's `name` is the natural row key)
- No options/checkboxes in v1 (backend has no force param)
- Error surfaces in the modal via `<ErrorDisplay>`; expect 409 `SECRET_IN_USE` with podman's message when a container references the secret
- Delete action visible only to `canManageSecrets` users

## Acceptance criteria

- [ ] Delete button hidden for read users; present per row for write/admin
- [ ] Confirmation modal shows the secret name; cancel aborts without a request
- [ ] Success (204) closes the modal and refetches the list
- [ ] 409 `SECRET_IN_USE` and 404 `SECRET_NOT_FOUND` render code + message in the modal
- [ ] MSW tests: request URL/method, cancel path, success invalidation, error envelope display, read-user hiding

## Blocked by

- 170-secrets-ui-01-list
