---
title: Secrets UI — tab, list, and read-only gating
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

Establish the frontend secrets module and fill the machine detail page's new **Secrets** tab with the machine's secret list: name, driver, created date per row. This slice mirrors the quadlets-tab precedent: a feature service (`services/secretsApi.ts`) calling the shared axios client against `GET /machines/${machineId}/secrets`, a `useSecrets(machineId)` React Query hook keyed `['machines', machineId, 'secrets']` so caches stay per-machine, and a `SecretList` component with distinct loading, empty, and error states (error via the shared `<ErrorDisplay>` with envelope code + message).

Add the `<Tab>Secrets</Tab>` + `<TabPanel><SecretList machineId={machine.id} /></TabPanel>` pair to `MachineDetailPage.tsx` between Quadlets and Settings (import alignment: Tab and TabPanel lists must stay index-aligned).

Read-only gating lands here once and carries through the series: add `canManageSecrets(permission)` (write+) and `canRevealSecrets(permission)` (admin) to `features/machines/permissions.ts`. `SecretList` uses `effectivePermission(machine)` and renders, for users below write, an explanatory hint in place of the create button — the SettingsTab hide-and-hint pattern, never disable-only. (Create/delete buttons arrive in later slices and must respect the same gate.)

## Acceptance criteria

- [x] Secrets tab lists secrets with name, driver, and created date
- [x] `useSecrets(machineId)` uses the query key hierarchy `['machines', machineId, 'secrets']`
- [x] Loading, empty, and error states are visually distinct; error shows the envelope code and message
- [x] Read-only users see a permission hint instead of write controls; write/admin see the create affordance area (button lands in 170-02)
- [x] MachineDetailPage tests cover the new tab and its panel mounting `SecretList`
- [x] Vitest + RTL + MSW tests cover: rows from a mocked list, empty list, error response, read-user hint vs write-user controls

## Blocked by

160-secrets-management backend endpoints (MSW mocks shapes; real backend for manual verification)

## Implementation notes (2026-08-28)

- `SecretList` takes `machineId` + `permission`; the page applies `effectivePermission(machine)` (mirroring SettingsTab's owner→admin handling) and the list gates on `canManageSecrets`.
- The read-user hint uses `data-testid="secrets-permission-hint"` (not SettingsTab's `permission-hint`): Chakra mounts all tab panels eagerly, so both hints are in the DOM at once and the shared testid broke `getByTestId` uniqueness in the page permission tests.
- Error-envelope test uses a 404 MACHINE_NOT_FOUND fixture: the shared `getApiError` deliberately collapses 5xx/internal codes to the generic message, so a 502 CONNECTION_FAILED body would not carry its code.
