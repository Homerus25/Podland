---
title: Secrets UI — create modal with masked value input
parent: 170-secrets-ui
date: 2026-08-27
status: closed
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

Add secret creation to the Secrets tab: a **Create secret** button (visible to `canManageSecrets` users only) opening a modal with two fields — **name** and **value**.

- Value control: Chakra `InputGroup` with `Input type="password"` plus an eye `InputRightElement` button toggling `type` between `password` and `text`. No third-party dependency.
- Validation before submit: name required matching `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` (max 255), value required (max 1 MiB guard is theoretical here; enforce non-empty). Inline field errors, matching the repo's form validation style.
- Submit calls `createSecret(machineId, { name, value })`; the name is trimmed, the value is sent **byte-identical** — **never** trimmed (backend trims the name too).
- Success closes the modal and invalidates `['machines', machineId, 'secrets']`; failures render via `<ErrorDisplay>` inside the modal (esp. 409 `SECRET_ALREADY_EXISTS`).
- Follow the quadlets create flow shape: modal state in `SecretList`, form resolves its submit promise `onSettled` so the button tracks pending state.

## Implementation notes (2026-08-28)

- The embedded-newline AC cannot hold at the DOM level: HTML value sanitization strips `\n` from single-line inputs (password AND text), verified against jsdom. The UI test proves the untrimmed transport with leading/trailing spaces; an api-layer test (`secretsApi.test.ts`) pins that whatever value the form produces is POSTed byte-identical, embedded newlines included. Multi-line values (PEM keys) would need a masked textarea — follow-up material.
- Name validation applies to the TRIMMED name (submit trims, so padding around a valid name must not trip the pattern); value validation rejects whitespace-only input because the masked control makes an accidental whitespace value undetectable, but a submitted value is never trimmed.
- Backend create endpoint is 160-03 (still open); MSW mocks its documented shape (`{name, value}` → 201 `{id}`, 409 `SECRET_ALREADY_EXISTS`).

## Acceptance criteria

- [x] Create button hidden for read users; modal reachable for write/admin
- [x] Value input renders masked; eye toggle reveals/hides
- [x] Name and value validation errors shown inline; submit disabled while invalid/pending
- [x] Payload contains the untrimmed value (test with leading/trailing spaces and an embedded newline)
- [x] 201 success closes modal and refetches list; 409 conflict message displayed
- [x] MSW tests: payload shape, masked rendering, validation, success and 409 flows

## Blocked by

- 170-secrets-ui-01-list
