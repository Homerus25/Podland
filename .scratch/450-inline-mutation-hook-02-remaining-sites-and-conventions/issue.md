---
title: The remaining non-modal sites migrate, mutationToPromise dies, and the written record catches up
parent: 450-inline-mutation-hook
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - refactoring
  - docs
author: vibe-agent
---

## Parent

`.scratch/450-inline-mutation-hook/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

Every remaining non-modal `useMutation` site drops its raw members and
rides the hook: the secret reveal (dismiss on close, run on reveal), the
quadlet lifecycle (dismiss at the options modal's open/close boundaries,
the modal consuming the wrapped slice, and the Start button gaining the
ritual it always lacked), the containers start/stop rows, and the machine
settings inline form (whose `mutationToPromise` bridge folds into `run`,
deleting the util). Then the written record catches up so the next agent
inherits the one-owner ruling instead of the copy-the-sibling ritual: the
frontend conventions' feature-local-resets clause becomes the hook rule,
the React Query section gains the hook bullet, the Tests section gains the
pinned-once clause, and the C6 settlement entry (now the dated
MutationDialog bullet in `docs/frontend-conventions.md`) is amended.
Users see exactly one difference, and it is the fix: a stale error never
survives into a retry's pending phase anywhere.

## Acceptance criteria

- [ ] `SecretDetailModal`: closing the modal dismisses the reveal mutation before `onClose`; the reveal button keeps its `setIsVisible(false)` and calls `run(secret.name)`; the value branch reads the hook's `data`; the error region reads the hook's `error`
- [ ] `QuadletList`: opening the lifecycle options modal dismisses that action's mutation and records the pending action; closing dismisses it and clears the state; `LifecycleOptionsModal` receives the wrapped slice and its confirm calls `run(input, { onSuccess: onClose })`; the Start button goes through `run`; `lifecyclePending` combines the wrapped hooks' `isPending`; the row's error sub-row reads the hook's `error`
- [ ] `ContainerList`: each row's start/stop action goes through `run`, and the error sub-row reads the hook's `error`
- [ ] `MachineSettingsTab`: the form submit becomes `(input) => updateMachine.run(input)` — the settle-promise keeps the form's submit state spanning the whole request; `submitError`, `isSubmitting`, and the "Changes saved." banner read the hook; the banner's semantics and every other behavior of the tab are unchanged
- [ ] `shared/utils/mutationPromise.ts` is deleted, its import is gone, and the `useListInvalidatingMutation` comment no longer mentions it
- [ ] Grep gate: no raw `.mutate(` or `.reset(` on a `useMutation` result anywhere in `features/` — the dialog modules' sanctioned structural slices are the only mutation-result consumers left
- [ ] `docs/frontend-conventions.md`: the line-30 clause ("Non-modal mutation sites … keep feature-local resets") is replaced by the rule that non-modal `useMutation` sites ride `useInlineMutation`, with the pull toolbar named as streaming-side and therefore outside it; the React Query section gains a bullet documenting the hook's invariant (`run` = reset+mutate, `dismiss` = reset, per-target pending); the Tests section gains the pinned-once clause pointing at the hook suite
- [ ] `docs/frontend-conventions.md`: the mutation-dialogs ruling's tail ("Non-modal mutation sites … keep feature-local resets" — the C6-dated bullet in Components) is amended to the new ruling, dated and attributed like the C9 entry ("Settled 2026-09-15 (architecture review candidate C12)")
- [ ] The full frontend suite is green (`npx vitest run`) — the migrations are behavior-preserving for everything the feature suites pin — and `npx biome check` passes on the touched files

## Blocked by

- `450-inline-mutation-hook-01-hook-and-share-tracer` (the hook and its pinned suite must exist before any further site can adopt them; the conventions text must not declare a reference that is not yet true in the code)

## Technical notes

- The settings form is the only site whose visible behavior changes, and only by the fix: the stale submit error clears during the next attempt's pending phase. If any settings test appears to pin the stale-error-through-pending behavior, that pin is the bug — change the test, not the hook.
- The reveal site's dismiss-on-close is also secret hygiene: it clears the revealed value from the mutation state, not just the error. Keep it even though the modal unmounts anyway.
- Do not fold `LifecycleOptionsModal` into `MutationDialog` — the parent settled it as inline residue (decision 5); it consumes the wrapped slice and nothing more.
