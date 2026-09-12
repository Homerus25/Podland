---
title: "NDJSON stream transport - one owner for every streaming fetch's auth, abort, supersede, and terminal rules"
parent: none
date: 2026-09-12
status: ready-for-agent
labels:
  - ready-for-agent
  - epic
  - frontend
  - shared
  - streaming
  - refactoring
  - testing
author: vibe-agent
---

## What to build

Every streaming endpoint in the frontend re-implements the same transport and
lifecycle ritual around the shared NDJSON parser: the Bearer preamble (plain
fetch, because axios buffers), the AbortController lifecycle, the supersede
guard (a new operation aborts the old; a superseded run's late events and
cleanup must not clobber the newer one), and the terminal-state rules
(complete/error events, HTTP failures, abort-is-not-an-error). The image pull
hook and the quadlet auto-update hook are near copies (~90 duplicated lines;
the pull hook's own comment says the pattern is deliberately non-specific so
later flows can follow it - the auto-update hook copy-pasted it instead), the
log-follow helper re-implements the fetch half, and the blob download helper
carries the Bearer preamble a fourth time.

After this epic one deep stream transport module owns all of it in two layers:
a `streamNdjson` function (the streaming fetch: auth, error envelopes, the
NDJSON consume loop) and a `useNdjsonOperation` hook on top (the operation
lifecycle: abort, supersede, exactly-one-terminal, running/error state). The
two operation hooks become thin adapters keeping only their event handlers;
log-follow collapses its fetch half onto the function layer. The next
streaming endpoint starts at zero.

Origin: architecture review 2026-09-12 candidate C1 ("One stream transport
for every NDJSON operation"), refined in a grilling session on 2026-09-12
(two rounds, all decisions settled). Slices implement these decisions; they
do not reopen them.

## Design decisions (settled in grilling)

1. **Two layers, one shared streaming module.** A `streamNdjson` plain
   function absorbs the streaming fetch ritual; a `useNdjsonOperation` React
   hook on top absorbs the operation lifecycle. Both layers earn their
   existence: log-follow is signal-driven with no supersede semantics and
   rides only the function layer; the operation hooks need both. The two
   operation hooks keep their public signatures and become adapters.

2. **The hook owns the terminal contract.** The backend's streaming
   orchestration guarantees exactly-one-terminal with a `complete`/`error`
   vocabulary for operation streams, so the hook enforces it: a value with a
   string `operationId` and type `complete` finishes the run; type `error`
   finishes it and sets the error state (falling back to the generic
   sentinel). Every value - terminal ones included - still reaches the
   caller's `onEvent` for feature side effects (invalidation, report
   capture), but only while this run is current and not aborted. The
   non-terminal vocabulary stays open (`progress` for pull, `log` for
   auto-update); shape guards remain feature-side and receive `unknown`.

3. **Scope: operations and log-follow; download stays out.** `usePullImage`
   and `useQuadletAutoUpdate` ride the hook; `followLogStream` keeps its
   exact signature and collapses its body onto `streamNdjson` (its body is
   the function layer). `downloadFile` is untouched: its duplication is a
   six-line Bearer preamble with no abort or supersede, and bytes do not fit
   an NDJSON-shaped interface. Forcing it through would widen the module's
   interface without absorbing complexity.

4. **Strictly behavior-preserving.** Public hook signatures, state names,
   warn messages, and test expectations stay; the duplication moves under one
   owner. The untouched existing suites are the acceptance gate throughout
   the migration; renames or behavior changes are separate, deliberate work.

5. **The `streamNdjson` contract:**

   ```ts
   streamNdjson(
     url: string,
     init: { method?: string; body?: string; signal: AbortSignal },
     onValue: (value: unknown) => void,
   ): Promise<void>
   ```

   Injects the Bearer header from the shared token holder; sets
   `Content-Type: application/json` only when a body is present (GET
   log-follow sends none today). `!ok` rejects with the envelope from
   `getApiErrorFromResponse`; a missing body rejects the generic sentinel.
   An aborted fetch or read rethrows the raw error (the lifecycle owner
   decides what an abort means); every other failure rejects as a generic
   ApiError - identical to what all three current consumers produce. Resolves
   on clean stream end; every parsed value reaches `onValue`.

6. **The `useNdjsonOperation` contract:**

   ```ts
   useNdjsonOperation(): {
     isRunning: boolean;
     error: ApiError | null; // error events + transport failures, never aborts
     start: (options: {
       url: string;
       init?: { method?: string; body?: string };
       onEvent: (event: unknown) => void;
     }) => void;
     cancel: () => void;
   }
   ```

   `start` is fire-and-forget (nothing awaits these today; terminal state is
   observable via `isRunning`): it aborts any in-flight run (supersede),
   resets `error`, sets `isRunning`. Unmount aborts - the current abortRef
   ritual moved verbatim. Features map the hook's state onto their own
   vocabulary (`isPulling`, `connectionError`) and keep their domain state
   (progress, lines, report, dryRun) and invalidation choices.

7. **Adjacent contractions.** The `VITE_API_BASE_URL || "/api"` constant is
   declared in four places (both operation hooks, both log API services);
   it is exported once from the shared api utils and the copies collapse.
   `consumeNdjsonStream` has exactly one caller (the pull hook) and is
   deleted once the migration leaves it dead; `StreamEvent`,
   `StreamEventState`, and `isStreamEvent` stay (the pull guard and the
   stream test fixtures use them).

8. **Tests: pin once, keep domain.** The module suite pins the transport
   invariants once - Bearer header, content-type-only-with-body, `!ok`
   envelope, no-body generic, abort (no error, request signal aborted,
   teardown), supersede (late events cannot clobber the newer run),
   exactly-one-terminal, events swallowed after abort. Feature suites keep
   domain assertions (progress/report exposure, invalidation including the
   dry-run skip, error-envelope routing, log-event filtering and error-event
   parsing) and thin their transport re-pins only in the final slice, after
   the whole migration is green - the gate never weakens mid-refactor.

9. **Expand-contract, chained.** The module lands beside the copies (expand),
   each adapter migrates as its own slice (each green), and the deletions
   and test thinning contract at the end. The chain is sequential even where
   slices are logically independent, keeping module evolution
   single-threaded - the same rhythm the ResourceList and MutationDialog
   epics used.

## Vertical Slices

### 1. The stream transport module and its pinned suite
**Issue**: 330-1-module-and-suite
- `streamNdjson` and `useNdjsonOperation` land in one new shared streaming
  module with the suite that pins every transport invariant once; nothing
  consumes them yet (the expand phase), the suite is the proof

**Status**: ready-for-agent
**Blocked by**: None (can start immediately)

### 2. The image pull hook as the first transport adapter
**Issue**: 330-2-pull-adapter
- `usePullImage` becomes a thin adapter over `useNdjsonOperation`, keeping
  its public shape, progress state, and images invalidation; its existing
  suite runs untouched and green

**Status**: ready-for-agent
**Blocked by**: 330-1

### 3. The quadlet auto-update hook as the second transport adapter
**Issue**: 330-3-auto-update-adapter
- `useQuadletAutoUpdate` becomes the second adapter, keeping its lines/
  report/dryRun state, its event guard, and the dry-run-skipping
  invalidation; its existing suite runs untouched and green

**Status**: ready-for-agent
**Blocked by**: 330-2

### 4. Log-follow onto the transport, apiBaseUrl dedupe, dead-code contract
**Issue**: 330-4-logstream-dedupe-cleanup
- `followLogStream` collapses onto `streamNdjson` keeping its signature; the
  apiBaseUrl constant is exported once; the now-dead `consumeNdjsonStream`
  is deleted

**Status**: ready-for-agent
**Blocked by**: 330-3

### 5. Conventions update and test thinning
**Issue**: 330-5-conventions-test-thinning
- frontend-conventions.md and CONTEXT.md record the stream-transport ruling;
  feature suites thin their module-owned re-pins, keeping domain assertions

**Status**: ready-for-agent
**Blocked by**: 330-4

Slices 2 and 3 are logically independent (different feature folders) but the
chain keeps module evolution single-threaded, mirroring the ResourceList and
MutationDialog epics.
