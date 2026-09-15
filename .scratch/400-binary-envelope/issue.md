---
title: Seal the binary envelope bypass: volumes export/populate (architecture review C8)
parent: none
date: 2026-09-15
status: open
labels:
  - needs-triage
  - backend
  - refactoring
author: vibe-agent
---

## Problem Statement

The volumes export and populate endpoints are the last paths that bypass
the endpoint envelope. Each hand-rolls the ritual `web.Handle` owns —
identity extraction, the AppError→envelope mapping, the 500 fallback —
and the copies speak a different vocabulary than every sealed path: an
identity miss answers 401 `ErrInvalidCredentials` here but 500
`INTERNAL_ERROR` everywhere the envelope owns (`web.Handle`,
`streaming.Begin`, and the buffered logs since issue 340), a missing
machine ID answers 400 `MISSING_MACHINE_ID` (a code doku never documents
for these endpoints), and AppError detection is a direct type assertion
that misses wrapped sentinels (`web.Handle` unwraps via `errors.AsType`).
One wiring bug, two vocabularies. Issue 340 sealed the buffered-logs
bypass under the ruling "the bypass is the friction, not the copy
count"; these two binary paths are what that ruling left out.

## Solution

A `web.HandleStream` twin of `web.Handle` in `internal/web` owns the
envelope's error-side for machine-scoped endpoints whose success is a
byte stream or multipart upload rather than a DTO: identity extraction
(the 500 invariant), AppError mapping, and the 500 fallback with its
log. The handlers keep only what is wire-specific — tar headers and the
streaming service call for export; the multipart ritual, the import
call, and the 204 for populate. The wire contract does not change: the
branches being sealed are wiring-bug branches doku never documented.

## Decisions (from the design session — do not relitigate)

1. **Seam shape**: `HandleStream(w, r, stream func(ctx, userID,
   machineID string) error)` lives in `internal/web`. No
   `successStatus`, no generic result — the closure owns headers, body
   ritual, service call, and the success write (implicit 200 for
   export, explicit 204 for populate); the envelope owns identity,
   AppError mapping, the 500 fallback, and failure logging.
2. **Unwrapping parity**: AppError detection uses `errors.AsType` like
   `web.Handle`, fixing the direct type assertion that missed wrapped
   sentinels.
3. **Identity contract** (issue 340 decision 4 applied verbatim): a
   missing userID or machineID is 500 `INTERNAL_ERROR` plus a loud log —
   the subtree middleware guarantees both, so a miss is a miswired
   route. The 401 `ErrInvalidCredentials` branch and both handler-level
   `MISSING_MACHINE_ID` branches are deleted; the middleware's own
   branch is untouched. `MISSING_VOLUME_NAME` stays a handler-owned 400
   returned from the closure, like `Get`/`Delete` inside `web.Handle`.
4. **Populate ritual placement**: the whole body ritual moves inside
   the envelope closure — `MaxBytesReader`, `MultipartReader`,
   `collectUpload` with its cleanup defer — and the `MaxBytesError` →
   `ErrVolumeUploadTooLarge` (413) translation folds into
   `collectUpload`, so its client errors all leave as AppErrors and the
   closure reduces to collect + import + 204.
5. **Logging**: the happy-path `"Export"`/`"Populate"` slog lines are
   dropped; failure logging is centralized in `HandleStream` with
   `Handle`'s exact shape (requestID + error; no volumeName context —
   the requestID correlates).
6. **Test split (pinned once)**: `internal/web/web_test.go` pins the
   twin's six behaviors — success passthrough (no JSON encode, no
   forced status), AppError mapping, wrapped-AppError unwrap,
   plain-error 500 fallback, and both identity invariants (closure must
   not run). The volumes suites thin by exactly two tests:
   `Export_MissingMachineID` (invariant re-pin) and `Export_ServiceError`
   (fallback re-pin; the header-replacement property stays pinned by
   `Export_NotFound`). Everything else — including all populate tests
   and `Export_NotFound`'s no-tar-header-leak pin — is unchanged, and
   the integration tests pass unchanged.
7. **doku unchanged**: the documented 401s are the middleware's
   `TOKEN_*` codes, which still apply; the sealed branches were never
   contract.

## Slices

- `400-binary-envelope-01-seam-and-volumes` — `web.HandleStream` with
  its pinned suite; both volumes endpoints cross the seam; the two
  re-pin tests die; integration green.
- `400-binary-envelope-02-conventions-update` — the backend conventions
  record the binary-envelope ruling (volumes export/populate the
  reference), `internal/web`'s package doc drops its JSON-only phrasing,
  `CONTEXT.md` gains the C8 entry, issue 340 gains the cross-link, doku
  verified unchanged.
