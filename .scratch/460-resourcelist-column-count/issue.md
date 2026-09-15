---
title: ResourceList hands rows the column count (architecture review C13)
parent: none
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - refactoring
  - testing
author: vibe-agent
---

## Problem Statement

The column count of a resource list is declared twice per list, and nothing
links the two declarations. Once as data — the `columns` array the feature
hands to `ResourceList`, which renders the header row from it
(`shared/components/ResourceList.tsx:88-95`) — and once as a bare literal
in the row's error sub-row: `<Td colSpan={5}>` in the containers rows
(`features/containers/components/ContainerList.tsx:148` against the 5
columns at `:178-184`) and `<Td colSpan={6}>` in the quadlet rows
(`features/quadlets/components/QuadletList.tsx:258` against the 6 columns
at `:421-428`). Add a column, forget the literal, and the error sub-row
renders short — silently, because no test asserts the width and the
misalignment is only visible when a row action fails.

The fragility is not hypothetical: `SecretList`'s column count is already
dynamic (3 or 4 depending on `canManage`,
`features/secrets/components/SecretList.tsx:121-126`). A future error
sub-row there cannot be written as a literal at all. Today the width is
one fact with two homes and no link; it should cross the seam instead.

## Solution

Widen the row seam: `ResourceList` derives the width once from its
`columns` data and hands it to every row — `renderRow(item,
{ columnCount })`. The two row components with error sub-rows
(`ContainerRow`, `QuadletRow`) take a required `columnCount` prop and span
their sub-rows with it; both literals are deleted. Adapters without
sub-rows do not change at all. The written record follows: the frontend
conventions' adapter contract gains the one line that makes the rule
durable, and the seam is pinned once in the shared suite, static and
dynamic width alike.

## Decisions (from the design session — do not relitigate)

1. **Fix shape: widen the seam, do not absorb the sub-row.** The error
   sub-row ritual exists in only 2 of 6 adapters — below the third-copy
   trigger this codebase applies — and its two error sources differ
   semantically (container lifecycle vs quadlet start). Handing rows the
   width is a one-token interface widening that covers every current and
   future adapter. If a third error sub-row ever appears, moving the
   ritual itself into `ResourceList` (a `renderRowError` seam) becomes the
   natural evolution and does not conflict with this widening. Rejected
   alternatives: per-list hoisted `columns` consts with
   `colSpan={columns.length}` (leaves the trap armed for the next
   adapter), and keeping the literals (leaves a silent layout bug).
2. **Seam shape (a type shape encodes this more precisely than prose):**

   ```ts
   renderRow: (item: T, context: { columnCount: number }) => ReactNode
   ```

   The second argument is an object bag — forward-compatible without
   another signature break — carrying exactly one key today. No index is
   added: rows carry their own keys by `ResourceList`'s existing contract.
3. **Name: `columnCount`.** The fact is the number of columns in the
   table; `colSpan` is a rendering attribute of the consuming sub-row and
   would smuggle the implementation into the interface.
4. **The row components take `columnCount` as a required prop.** No
   default — a default would be the deleted literal, relocated. Required
   means TypeScript forces every future call site of `ContainerRow` and
   `QuadletRow` to thread the width; the link is non-optional at both
   ends of the seam.
5. **Migration scope: only the two adapters with sub-rows change.** The
   containers and quadlets list lambdas become
   `(item, { columnCount }) => <Row columnCount={columnCount} .../>`; the
   literals die. The four adapters without sub-rows (volumes, images,
   secrets, the shared suite's gadget row) stay untouched — a
   single-parameter lambda remains type-correct under the widened seam,
   and touching them is churn with no invariant gained.
6. **The conventions line lands in the adapter contract paragraph**
   (`docs/frontend-conventions.md`, the `ResourceList` contract entry):
   `renderRow` receives `{ columnCount }`; a row's error sub-row spans
   `columnCount`, never a literal. A contract change without its
   convention line is how the next literal reappears.
7. **The seam is pinned once, in the shared suite** — per the
   conventions' own pinned-once rule for `ResourceList` behavior
   (`shared/components/ResourceList.test.tsx`): `renderRow` receives
   exactly `columns.length`, one static-width case and one dynamic-width
   case (a conditional action column, the `SecretList` shape that
   motivated this issue). Feature suites keep their existing assertions
   unchanged; none pins widths today, and the shared pin makes drift
   structurally impossible, so re-pinning per feature gains nothing.

## Out of Scope

- Owning the error sub-row ritual inside `ResourceList` (see decision 1:
  the third-copy evolution, not this change).
- Any change to the four adapters without error sub-rows, including
   mechanical lambda updates for uniformity.
- New width assertions in the containers or quadlets feature suites.

## Further Notes

- Architecture review candidate C13 ("ResourceList hands rows the column
  count", rated worth exploring, in-process). The review's cited paths
  predate the `features/` layout; line references above are current.
- Exactly two `colSpan` occurrences exist frontend-wide (the two literals
  above) — the audit is exhaustive, no third site lurks.
