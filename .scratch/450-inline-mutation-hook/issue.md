---
title: useInlineMutation owns the non-modal reset ritual (architecture review C12)
parent: none
date: 2026-09-15
status: open
labels:
  - needs-triage
  - frontend
  - shared
  - refactoring
author: vibe-agent
---

## Problem Statement

The invisible invariant "reset before mutate, reset on dismiss" — clear the
stale error so the next attempt starts clean — is hand-rolled at six
`.reset()` call sites in three features: the share grant and revoke
submissions (`ShareMachineSection.tsx:36`, `:69`, the revoke one also
hand-rolling per-target pending via `variables === grant.userId`), the
secret reveal (`SecretDetailModal.tsx:55` on close, `:132` before mutate),
and the quadlet lifecycle options modal's open/close boundaries
(`QuadletList.tsx:166`, `:171`). Nothing enforces the ritual: a site that
forgets it compiles fine, and no test pins it — the invariant is invisible
in the code and in the suites.

The drift is already here: the quadlet Start button (`QuadletList.tsx:192`)
and the containers start/stop rows (`ContainerList.tsx:118`) call `mutate`
bare, so a stale error survives a retry's pending phase; the machine
settings inline form (`MachineSettingsTab.tsx:85`) shows its submit error
with no reset at all. This is exactly the residue the MutationDialog
settlement (issue 320, candidate C6) left outside dialogs when it ruled
that non-modal sites keep feature-local resets — at six ritual sites plus
three bare-mutate sites across five features, that clause is worth
reopening.

Out of scope by shape, not by omission: volumes populate rides
`MutationDialog` (dialog-owned reset), and the pull-image toolbar is
streaming (`useNdjsonOperation`), outside the `useMutation` world entirely.

## Solution

A `useInlineMutation` hook in `shared/queries` wraps a mutation result and
owns the ritual: `run` is reset+mutate, `dismiss` is reset, and the raw
`mutate`/`reset` members are hidden so the invariant cannot be bypassed.
Every non-modal `useMutation` site in the features migrates to it — the
five ritual sites and the three bare-mutate drift sites alike, so the
rewritten convention starts with zero exceptions. The settings form's
promise bridge (`mutationToPromise`, single caller) folds into `run` and
the util dies. The written record follows: the frontend conventions'
feature-local-resets clause becomes the hook rule, the invariant gets its
pinned-once suite, and `CONTEXT.md`'s C6 settlement entry is amended.

## Decisions (from the design session — do not relitigate)

1. **Scope: every non-modal `useMutation` site, one sweep.** In: share
   grant, share revoke, secret reveal, quadlet stop/restart (the options
   modal's open/close boundaries), quadlet Start, containers start/stop
   rows, the machine settings inline form. Out: everything behind
   `MutationDialog`/`ConfirmDialog`/`PruneReportDialog` (dialog-owned) and
   the pull toolbar (streaming, not `useMutation`). A migration that left
   the known bare-mutate sites behind would re-create "enforced nowhere"
   on day one.
2. **Hook surface (a type shape encodes this more precisely than prose):**

   ```ts
   function useInlineMutation<TData, TError, TInput>(
     mutation: UseMutationResult<TData, TError, TInput>,
   ): {
     run(input: TInput, options?: { onSuccess?: (data: TData) => void }): Promise<void>;
     dismiss(): void;
     error: TError | null;
     isPending: boolean;
     isPendingFor(input: TInput): boolean;
     data: TData | undefined;
     isSuccess: boolean;
   }
   ```

   The raw `mutate`/`reset` are not exposed — the ritual is unbypassable,
   the deep-module move. `data` (the revealed secret) and plain `isPending`
   (the grant button, the quadlet `lifecyclePending` combine) and
   `isSuccess` (the settings saved banner) are load-bearing at real sites,
   not speculative surface.
3. **`run` unifies the two call shapes.** Reset, then mutate with the
   optional `onSuccess` passthrough, resolving on settle (never rejecting —
   failure surfaces through `error`). The lifecycle options modal's
   `mutate(input, { onSuccess: onClose })` and the settings form's
   settle-spanning submit promise both ride it; `mutationToPromise`
   (`shared/utils/mutationPromise.ts`, single caller) is deleted, along
   with its mention in the `useListInvalidatingMutation` comment.
4. **`isPendingFor` keeps reference/primitive equality.** It is today's
   revoke hand-roll (`variables === grant.userId`) made shared: exact match
   against the in-flight input. Object-input sites use plain `isPending`.
5. **The lifecycle options modal consumes the wrapped slice.** Open is
   `dismiss` + set the pending action, close is `dismiss` + clear it, and
   the modal's confirm calls `run(input, { onSuccess: onClose })` against
   the slice `{ run, isPending, error }`. The modal is inline residue, not
   dialog residue — it does not fold into `MutationDialog` (C6 stays
   settled).
6. **Settings UX is otherwise unchanged.** The one visible behavior change
   is the fix itself: a stale submit error clears during the next attempt's
   pending phase. The "Changes saved." banner keeps its semantics
   (`isSuccess` until the next submit); no new dismiss interaction is
   invented.
7. **The invariant is pinned once, in the hook's suite.** New
   `shared/queries/useInlineMutation.test.tsx` (renderHook, fresh
   QueryClient): stale error cleared when `run` starts, `dismiss` clearing
   error and data, `isPendingFor` true only for the in-flight input, `run`
   resolving on settle for both outcomes, the `onSuccess` seam firing
   exactly once. No feature suite pins the ritual today, so nothing is
   thinned — feature suites keep their domain assertions untouched.
8. **Home: `shared/queries/useInlineMutation.ts`**, beside
   `useListInvalidatingMutation` — the established home for shared
   React Query hooks; no new `shared/hooks` directory for one file.

## Slices

- `450-inline-mutation-hook-01-hook-and-share-tracer` — the hook with its
  pinned suite, and the share section (both rituals plus `isPendingFor`)
  converted as the tracer adapter.
- `450-inline-mutation-hook-02-remaining-sites-and-conventions` — secret
  reveal, quadlet lifecycle + Start, containers rows, and the settings form
  migrate; `mutationToPromise` dies; the conventions, Tests clause, and
  `CONTEXT.md` amendments land.
