---
title: The useInlineMutation hook and its pinned suite, with the share section as the tracer adapter
parent: 450-inline-mutation-hook
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - machines
  - testing
author: vibe-agent
---

## Parent

`.scratch/450-inline-mutation-hook/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The hook lands in `shared/queries` with exactly the surface the parent
decided — `run(input, { onSuccess? })` as reset+mutate returning a
settle-promise, `dismiss`, `error`, `isPending`, `isPendingFor`, `data`,
`isSuccess`, raw `mutate`/`reset` hidden — plus the suite that pins the
reset ritual once, so the invariant stops being invisible. The share
section converts as the tracer adapter: the richest site, carrying both
ritual shapes (inline submit, row action) plus the per-target pending
scoping. For a user nothing changes in the happy path — but a failed grant
or revoke no longer leaves its stale error on screen during the next
attempt's pending phase, and from this slice on the next non-modal site
starts at zero instead of copy-pasting the ritual.

## Acceptance criteria

- [ ] `shared/queries/useInlineMutation.ts` exports the hook with the parent-decided surface; it wraps an existing `UseMutationResult` (generic over data, error, input) and exposes no `mutate`, no `reset`, and nothing else beyond the seven members
- [ ] `run(input, options?)` calls reset and then mutate in that order, passes the optional `onSuccess` through to the mutation, and returns a `Promise<void>` that resolves when the mutation settles — on success and on failure alike; it never rejects
- [ ] `dismiss()` resets the mutation (error, data, and success state all clear)
- [ ] `isPendingFor(input)` is `isPending && variables === input` — exact reference/primitive equality, documented as such; object-input sites are expected to use plain `isPending`
- [ ] `shared/queries/useInlineMutation.test.tsx` pins the invariant once via `renderHook` with a fresh retry-free `QueryClient`: a stale error is cleared the moment `run` starts, `dismiss` clears error and data, `isPendingFor` is true only for the in-flight input, `run` resolves on settle for both outcomes, and the `onSuccess` seam fires exactly once per successful run
- [ ] `ShareMachineSection` migrates: the grant submit and each revoke click go through `run`, the revoke button's per-row spinner rides `isPendingFor(grant.userId)`, the grant button's spinner rides `isPending`, both error regions read the hook's `error`, and no raw `.mutate(` or `.reset(` remains in the file
- [ ] The existing `ShareMachineSection` suite passes unchanged — the swap is behavior-preserving for everything it pins (grant/revoke flows, pending during request, error envelopes, refetch)
- [ ] No other feature is touched in this slice; the remaining sites migrate in `450-inline-mutation-hook-02`
- [ ] The full frontend suite is green (`npx vitest run`) and `npx biome check` passes on the touched files

## Blocked by

- None (can start immediately)

## Technical notes

- The reset-before-mutate order is the whole point: mutating first would leave the previous attempt's error on screen through the new attempt's pending phase. Pin the order in the suite, not just the outcome.
- The settle-promise resolves rather than rejects by design — failure already has a home in `error`; a rejecting `run` would push sites into try/catch rituals the hook exists to remove.
