---
title: Extract the machine Settings tab from the page into the machines feature
date: 2026-09-12
status: open
labels:
  - ready-for-agent
  - frontend
  - machines
  - refactoring
  - testing
author: vibe-agent
---

## What to build

For a user nothing changes: the machine detail page keeps its tabs, the
Settings tab keeps its form, sharing section, delete button, and permission
hints, byte for byte. What changes is locality. The Settings tab stops being
feature logic hand-rolled inside `pages/MachineDetailPage.tsx` and becomes
`MachineSettingsTab`, a component in `features/machines/components/` beside
its siblings (MachineForm, ShareMachineSection, PermissionBadge), and every
suite that pins settings/sharing/delete behaviour moves with it, targeting
the component directly instead of mounting the whole page. After this, the
page is a shallow orchestrator again (identity header + tabs), settings
changes stop touching `pages/`, and machine behaviour finally has a home in
the machines feature.

Comes from the architecture review (C5). The review suggested landing this
with epic 320 slice 03; that slice has already landed (machine delete is a
ConfirmDialog consumer), so this is unblocked and independent.

## Decisions (settled in grilling - do not relitigate)

- One file: `MachineSettingsTab` carries its whole tab body; the delete
  section stays a private function inside the same file (single consumer,
  mirrors today's page-file shape). No separate DeleteMachineSection file.
- Props are `{ machine: Machine }`, nothing else: the tab computes
  `effectivePermission(machine)` internally, exactly as today. No interface
  change anywhere else.
- Tests are rewritten to direct mount, not moved verbatim: each migrated
  suite renders the component (not the page) in a fresh `QueryClient`
  wrapper with MSW overrides, per the sibling-feature pattern
  (SecretList/ContainerList suites). The delete suite additionally needs a
  router wrapper because delete success navigates to `/machines`.
- The one page-behaviour test that stays with the page: deriving write
  access from the machines list when the detail response omits permission
  (that merge lives in the `useMachine` hook surfaced through the page).
- Include the micro-cleanup: the page hoists its four inline
  `effectivePermission(machine)` calls into one local const for the list
  tabs. Nothing else is in scope (no header extraction, no list prop
  changes).
- No `doku/` update: no API contract changes.

## Acceptance criteria

- [ ] `features/machines/components/MachineSettingsTab.tsx` exports `MachineSettingsTab({ machine })`; its body is today's `SettingsTab` unchanged (permission gate with the `permission-hint` testids and `data-permission` values, the `machine-saved` banner, MachineForm in edit mode with `mutationToPromise` bridging, ShareMachineSection behind `canShareMachine`, delete behind `canDeleteMachine`), with the delete section private inside the file
- [ ] `MachineDetailPage.tsx` renders `<MachineSettingsTab machine={machine} />` as the Settings tab body and computes `effectivePermission` once per render for the four resource-list tabs; no settings/sharing/delete logic remains in the page
- [ ] The delete suite lives beside the component as `MachineSettingsTab.delete.test.tsx`, direct-mounted: cancel sends nothing, delete navigates away on success, failure renders the error in-modal and keeps the dialog open, machines list refetch asserted through MSW
- [ ] The edit suite lives beside the component as `MachineSettingsTab.edit.test.tsx`, direct-mounted: saved banner, update request shape, failure envelope on the form
- [ ] The sharing suite becomes `ShareMachineSection.test.tsx` beside its component, direct-mounted: grants list, grant/revoke flows and refetch, failure envelopes
- [ ] The permissions suite splits: settings-tab gating (admin sees all, write edits but sees the share/delete hint, read sees the read-only hint) becomes `MachineSettingsTab.permissions.test.tsx` direct-mounted; the pure permissions-module tests move to `features/machines/permissions.test.ts`; the page keeps only its page-level gating (secrets tab hint, derive-write-from-list)
- [ ] `MachineDetailPage` keeps only orchestration suites: loading, error envelope, identity header with badge, tab switching, one Settings tab smoke test, and the derive-write-from-list test
- [ ] No machine suite left in `pages/` mounts the page to reach settings behaviour; no `MachineDetailPage.*.test.tsx` file remains for delete/edit/share/permissions
- [ ] No behaviour change: same testids, same copy, same routing on any user path
- [ ] The full frontend suite is green (`npx vitest run`, under 2 minutes) and `npx biome check` passes on the changed files

## Blocked by

- None (can start immediately)
