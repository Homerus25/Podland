---
title: machine-resource query module — one owner for the React Query key vocabulary
parent: none
date: 2026-09-08
status: ready-for-agent
labels:
  - frontend
  - refactor
  - react-query
author: vibe-agent
---

## Problem Statement

The React Query key scheme `['machines', machineId, resource]` is a convention, not a module. Every feature constructs its keys privately: only `volumesQueryKey` is exported, quadlets/containers/secrets/images keep module-private constructors, and the machines feature uses raw inline arrays throughout. So when a quadlet auto-update finishes, its complete-handler invalidates quadlets, containers, and images by **hardcoding other features' key strings** — renaming a key can strand a string literal in another feature, and "what does finishing X invalidate?" has no single answer.

The same pressure produced three divergent local fixes to one problem: `useQuadletMutation` (generic input), `useContainerLifecycle` (fixed container-id input), and a hand-rolled `onSuccess` on `useCreateContainer` beside the very wrapper it duplicates, plus machines' `useInvalidateSharing` callback. The image-pull streaming hook hand-rolls its own-feature invalidation the same way.

This is candidate 3 of the 2026-09-04 architecture review. The design below was settled in a grilling session on 2026-09-08; the inline surface is that session's decision, not a suggestion.

## Solution

One module, `shared/queries`, owns the machine-resource key vocabulary and the list-invalidation behaviour. The key scheme itself is unchanged — this implements the existing conventions ruling, it does not revise it.

**Key vocabulary** (`shared/queries/queryKeys.ts`):

- `MachineResource = 'containers' | 'volumes' | 'images' | 'quadlets' | 'secrets'` — exactly the invalidatable set (it mirrors the machine detail page's resource tabs).
- Plural list constructors per resource, e.g. `containersQueryKey(machineId)` → `['machines', machineId, 'containers'] as const`.
- Machines' list key is the constant `machinesQueryKey = ['machines'] as const` (not a zero-arg function).
- Module-owned detail constructors where detail queries exist: `quadletQueryKey`, `secretQueryKey`, `volumeQueryKey` (each `(machineId, id)`), and `machineDetailKey(id)` — features stop composing `[...listKey, id]` by hand.
- Sub-resource constructors `machineStatusQueryKey(id)` and `machineAccessQueryKey(id)` reuse the same shape but are **not** `MachineResource` members — they are never invalidation targets.
- Plain `as const` readonly tuples, no branded types: all invalidation is prefix-based and the union already carries the rename-safety that matters.

**Invalidation** (`shared/queries/useListInvalidatingMutation.ts`):

- `useListInvalidatingMutation<TInput>(machineId, resources: MachineResource | MachineResource[], run: (input: TInput) => Promise<unknown>)` — returns the `useMutation` result untouched, so `isPending`/`error`/`mutate` and `mutationToPromise` all pass through. Common case: `useListInvalidatingMutation(machineId, 'secrets', deleteSecret)`.
- `invalidateMachineResources(queryClient, machineId, resources: MachineResource | MachineResource[]): void` — fire-and-forget, for non-mutation flows. The factory's `onSuccess` **delegates to it**: one invalidation code path.
- Streaming hooks call it directly: quadlet auto-update invalidates `['quadlets', 'containers', 'images']` behind its existing dry-run guard; image pull invalidates `'images'`.

**Migration is big-bang, all features at once.** The halves don't interoperate — mixed old/new regimes is the worst state. Machines, containers, quadlets (+ auto-update), images (+ pull), volumes, and secrets all adopt the constructors in one change; `useQuadletMutation`, `useContainerLifecycle`, and the hand-rolled `useCreateContainer` fold into the factory; `volumesQueryKey`'s two existing importers re-point. Machines' deliberate broad `['machines']` prefix invalidation stays as-is via the constant. Non-invalidating mutations (`useRevealSecret`) stay plain `useMutation`.

## Testing

- One factory-level test (`renderHook` + fresh `QueryClient`) owns the invalidation-on-success behaviour, with the codebase's single sanctioned `vi.spyOn(queryClient…)` on `invalidateQueries`. It asserts: success invalidates the own-resource key; the array form invalidates each; **a rejection does not invalidate**; the returned object is the passthrough mutation result.
- Feature tests drop their per-feature invalidation spies but keep their MSW refetch assertions untouched — refetch-through-the-public-interface is the seam that matters.
- Streaming-hook tests keep their invalidation assertions: the dry-run skip is bespoke logic nothing else covers.
- Gates per conventions: affected files during the work, full `npx vitest run` + `npx biome check` on changed files at the end.

## Documentation

`docs/frontend-conventions.md` only: the React Query section's two ruling bullets are replaced (keys come from `shared/queries` constructors; list-invalidating mutations go through the factory, cross-feature/streaming invalidation through the helper; a new resource tab adds one union member plus one list constructor), and the Tests section gains one line (invalidation asserted once at the factory). No CONTEXT.md change — nothing domain-level moved.

## Out of Scope

- **Query-hook factory (`useResourceList`)** — rejected in the session: query hooks vary (`gcTime`, `staleTime`, `enabled`) and a factory would grow an options bag mirroring `useQuery`. Keys + mutation factory only.
- **Streaming transport unification** (`useStreamOperation` / `openAuthorizedStream`) — candidate 4 of the same review; touches the same two hooks but is the transport seam, not the query vocabulary.
- **Machines' broad `['machines']` invalidation semantics** — deliberate and unchanged; it only gains the shared constant.
- **Reveal-secret caching ruling** — stays plain `useMutation` by design.
- **Backend** — untouched; no API contract changes.

## Slices

None — deliberately a single coherent change (see big-bang ruling above). Blocked by nothing; can start immediately.

## Acceptance criteria

- [ ] `shared/queries` exports the settled vocabulary: the `MachineResource` union, five plural list constructors, the `machinesQueryKey` constant, the four detail constructors, and the two sub-resource constructors
- [ ] `useListInvalidatingMutation` and `invalidateMachineResources` exist with the settled signatures; the factory's `onSuccess` delegates to the helper
- [ ] No feature file constructs a `['machines', …]` key array or a private key constructor; every key and invalidation flows through `shared/queries`
- [ ] `useQuadletMutation`, `useContainerLifecycle`, and the hand-rolled `useCreateContainer` invalidation are gone, folded into the factory; `useInvalidateSharing` uses the access constructor
- [ ] Quadlet auto-update and image pull invalidate via `invalidateMachineResources` (auto-update still gated on non-dry-run); machines' broad invalidation behaviour is unchanged
- [ ] The factory test pins success-single, success-array, rejection-invalidates-nothing, and passthrough result; per-feature invalidation spies are gone while MSW refetch assertions remain and pass
- [ ] `docs/frontend-conventions.md` carries the new ruling text and the one-line Tests addition; CONTEXT.md is untouched
- [ ] Full `npx vitest run` and `npx biome check` on changed files are green
