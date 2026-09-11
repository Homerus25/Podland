---
title: Secrets UI — list, create, delete, inspect with admin reveal
parent: none
date: 2026-08-27
status: open
labels:
  - needs-triage
  - frontend
  - secrets
  - feature
author: vibe-agent
---

## Problem Statement

As a user managing machines through Podland, I cannot see which podman secrets exist on a machine, create new ones, or clean up stale ones from the browser — every secret operation currently means SSH. Secrets gate credentials for containers and quadlets, so they are part of the day-to-day management surface.

## Solution

Add a **Secrets** tab to the machine detail page with the core secret surface: list with metadata (name, driver, created), create via a masked value input, delete with confirmation, and an inspect detail view whose admin-only **reveal** shows the secret value. Write actions are hidden for read-only users following the SettingsTab permission-hint pattern. Backed by the 160-secrets-management backend endpoints.

## User Stories

1. As a user with read access, I want to see all secrets on a machine with name, driver, and created date, so that I can survey what credentials exist.
2. As a user, I want distinct loading, empty, and error states on the secrets list, so that the tab is never ambiguously blank.
3. As a user with write access, I want to create a secret with a name and a masked value input (with reveal toggle), so that shoulder-surfers can't read the value.
4. As a user, I want the value I type to arrive byte-identical (no trimming), so that credentials with meaningful whitespace keep working.
5. As a user, I want required-field and name-format validation before submit, so that malformed input never reaches the server.
6. As a user with write access, I want to delete a secret after an explicit confirmation, so that cleanup is never accidental.
7. As a user, I want backend errors (in-use secret, name conflict, connection failure) surfaced with code and message, so that failures are diagnosable.
8. As an admin/owner, I want to inspect a secret's detail (ID, labels, driver, timestamps) and reveal its value, so that I can verify what a secret actually contains.
9. As an admin, I want revealed values masked by default with an eye toggle, so that revealing is a deliberate act.
10. As a read-only user, I want write controls hidden with an explanatory hint instead of disabled buttons, so that the UI states my access level honestly.

## Implementation Decisions

- New feature slice `src/features/secrets/` mirroring quadlets: `services/secretsApi.ts` (DTOs colocated), `hooks/useSecrets.ts` (query key `['machines', machineId, 'secrets']`, mutations invalidate the list key), `components/SecretList.tsx`.
- Tab added to `MachineDetailPage.tsx` between Quadlets and Settings; placeholder-free since it lands complete.
- Value input: Chakra `InputGroup` with `type="password"` + eye button (no third-party dep); no trim on submit.
- Permission predicates in `features/machines/permissions.ts`: `canManageSecrets` (write+), `canRevealSecrets` (admin). Gate components follow the SettingsTab hide-and-hint pattern; backend remains the enforcement point.
- Delete: confirmation modal pattern from `DeleteQuadletModal` (red confirm, `data-testid="confirm-delete-button"`).
- Inspect detail: expandable row or modal seeded from the inspect endpoint; reveal button calls `?reveal=true` and displays masked with toggle; value never persisted in React Query cache (detail query disabled `gcTime` like quadlets).
- No SSE/live updates (frontend event bus doesn't exist yet); React Query invalidation after mutations.

## Testing Decisions

- Vitest + RTL + MSW, prior art: quadlets suite.
- List tests: rendering rows, empty, error envelope, read-user hint replaces create/delete.
- Create tests: validation messages, masked input renders `type="password"`, payload untrimmed (value with spaces/newline), 409 conflict display.
- Delete tests: confirmation flow, request URL, list invalidation.
- Inspect/reveal tests: detail fields; `read`/`write` users never see reveal; admin reveal request carries `reveal=true`; returned value displayed masked with toggle.
- MachineDetailPage test gains a Secrets tab assertion.

## Out of Scope

- Labels, driver/driver-opts, replace/recreate flow (backend deferred them too).
- Secret usage indicators ("in use by container X").
- Referencing secrets from the container/quadlet create forms.
- Live updates via SSE events (podman emits `secret` events; revisit with a frontend event bus).

## Further Notes

- Blocked by the `160-secrets-management` backend series (endpoints must exist for MSW-free dev and E2E honesty; MSW mocks the shapes regardless).
- The permission matrix behind the UI: read → list + metadata inspect; write → +create/delete; admin → +reveal. Retrofitting button-level gating onto containers/quadlets tabs (frontend side of 160-00) is deliberately not folded in — separate follow-up.
