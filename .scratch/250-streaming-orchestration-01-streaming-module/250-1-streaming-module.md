--- 
title: Streaming Module — Begin, Shared Error Mapping, Config Change
parent: route every streaming endpoint through one orchestration
date: 2026-09-08
status: ready-for-agent
labels:
  - streaming
  - backend
  - module
author: vibe-agent
---

## Parent

Part of the "route every streaming endpoint through one orchestration" epic (`250-streaming-orchestration/250-master-streaming-orchestration.md`).

## What to build

Turn the `streaming` module into the full entry envelope for NDJSON endpoints, so later slices can migrate handlers onto it without any further module work:

- A `Begin(w, r)` entry point that performs the hand-copied ritual once: NDJSON headers, writer creation (writing the 500 `STREAMING_NOT_SUPPORTED` early error itself when flushing is unsupported), and identity extraction. It returns a stream handle carrying the NDJSON writer, userID, and machineID. Missing userID or machineID maps to 500 `INTERNAL_ERROR` plus a loud log line — mirroring the `web.Handle` envelope's ruling that a missing identity on the machine subtree is a wiring bug, not a client error.
- Move the AppError→terminal-event mapping into `Run`: the stream config gains the domain's default error sentinel, and the handler-supplied error-event builder receives already-mapped values instead of the raw error:

```
StreamConfig gains:    ErrorDefault *appErrors.AppError   // domain's fallback sentinel
ErrorEvent becomes:    func(code, message string, cancelled bool) any
```

- Mapping rules, owned by the module: `cancelled` → the `OPERATION_CANCELLED` code and message; a (possibly wrapped) `*AppError` → its own code and message; any other error → the `ErrorDefault` sentinel's code and generic message, with the raw cause logged once via `slog` (causes stay out of wire payloads per the cause-logging ruling).
- Log streams use `Result = struct{}`; a clean operation end fires the complete event. The module exports the shared end-of-stream vocabulary constant: level `INFO`, message `STREAM_ENDED`.

The two existing `Run` callers (image pull, quadlet auto-update) must keep compiling and behaving identically: adapt their error-event builders to the new mapped signature mechanically, leaving their entry ritual untouched — the full `Begin` refactor is ticket 250-4.

## Acceptance criteria

- [ ] `Begin` sets headers, builds the writer, and returns writer + userID + machineID; on unsupported flushing it writes the 500 `STREAMING_NOT_SUPPORTED` error response itself
- [ ] `Begin` responds 500 `INTERNAL_ERROR` with a loud log when userID or machineID is missing (identical ruling to `web.Handle`)
- [ ] `Run` maps cancelled / AppError / unknown errors per the rules above; handlers' error-event builders receive `(code, message, cancelled)` only
- [ ] Non-AppError failures produce the `ErrorDefault` sentinel's code and generic message in the terminal event, and the raw cause appears exactly once in server logs
- [ ] The `STREAM_ENDED` vocabulary constant exists at `INFO` level
- [ ] Module tests pin, once: the identity 500, the unsupported-flushing 500, and a mapping table covering cancelled, plain AppError, wrapped AppError, and unknown errors
- [ ] The existing `Run` invariant tests (exactly-one-terminal, disconnect, write-failure, completion) still pass unchanged
- [ ] Image pull and quadlet auto-update compile against the new config with wire behaviour unchanged

## Blocked by

None - can start immediately

## Technical notes

- The existing `Run` tests already pin exactly-one-terminal, disconnect cancellation, and write-failure exits; extend, don't duplicate.
- The mapping must unwrap wrapped errors (`fmt.Errorf("%w", …)`) the same way the JSON envelope does.
- This is the expand step: no endpoint migrates onto `Begin` here. Keeping the build green with the two existing callers adapted is what lets 250-2/3/4 run as independent slices.
