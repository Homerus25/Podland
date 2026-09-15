---
title: web.HandleStream and both volumes endpoints crossing the seam
parent: 400-binary-envelope
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - backend
  - web
  - volumes
  - testing
author: vibe-agent
---

## Parent

`.scratch/400-binary-envelope/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The endpoint envelope gains its binary twin and the two volumes
endpoints cross it. `web.HandleStream` in `internal/web` runs the
envelope's error-side for machine-scoped endpoints whose success is a
byte stream or multipart upload rather than a DTO, and `web_test.go`
pins its six behaviors once. Export becomes tar headers plus the
streaming service call inside one closure; Populate becomes the
multipart ritual, the import call, and the 204 inside one closure, with
the 413 translation folded into `collectUpload`. The hand-rolled
identity and error-mapping branches — the 401 `ErrInvalidCredentials`
answer, the handler-level `MISSING_MACHINE_ID` 400s, the direct type
assertions, the per-handler slog — are deleted. For a user nothing
changes: same tar download, same merge upload, same documented error
codes; but an identity miss now answers 500 everywhere the envelope
owns, and the last bypass is sealed.

## Acceptance criteria

- [ ] `internal/web` exports `HandleStream(w http.ResponseWriter, r *http.Request, stream func(ctx context.Context, userID, machineID string) error)` — no `successStatus`, no generic result; the closure owns headers, body ritual, service call, and the success write
- [ ] `HandleStream` behavior: a missing userID or machineID answers 500 `INTERNAL_ERROR` with the loud invariant log and the closure never runs; a returned AppError — including a wrapped sentinel, detected via `errors.AsType` — answers its own status/code; any other error answers the 500 fallback with `Handle`'s log shape (requestID + error)
- [ ] `web_test.go` pins all six behaviors once: success passthrough (the closure's headers, status, and bytes reach the response unmodified — no JSON encode, no forced status), AppError mapping, wrapped-AppError unwrap, plain-error 500 fallback, missing userID, missing machineID
- [ ] Export crosses the seam: the handler is tar headers + `service.Export` in one closure; the 401 branch, the `MISSING_MACHINE_ID` branch, the type-assert mapping, and the `"Export"` slog line are deleted; `MISSING_VOLUME_NAME` stays a 400 `NewAppError` returned from the closure
- [ ] Populate crosses the seam: `MaxBytesReader`, `MultipartReader`, `collectUpload` (cleanup defer included), `service.Import`, and the 204 write all live in the closure; `MaxBytesError` → `ErrVolumeUploadTooLarge` is folded into `collectUpload`, whose client errors now all leave as AppErrors; the handler-level error-mapping branches and the `"Populate"` slog line are deleted
- [ ] The export suite keeps Success, NotFound (the no-tar-header-leak pin), and MissingVolumeName unchanged; `TestVolumeHandler_Export_MissingMachineID` and `TestVolumeHandler_Export_ServiceError` are deleted as envelope re-pins
- [ ] The populate suite is unchanged and green (FilesShape, ArchiveShape, ShapeValidation, TraversalRejected, CapEnforced, NotFound)
- [ ] `Test_Integration_Volumes_Export`, `Test_Integration_Volumes_Export_AccessControl`, and `Test_Integration_Volumes_Populate` pass unchanged
- [ ] The backend suite is green and touched files pass `gofmt`

## Blocked by

- None (can start immediately)

## Technical notes

- Setting the tar headers before the service call and letting the error envelope's `Set` replace `Content-Type` is today's behavior; `Export_NotFound` pins it. Keep the handler doc comment's ordering contract (existence resolves before the first byte is written).
- Do not touch conventions, `CONTEXT.md`, or doku in this slice; they ride in `400-binary-envelope-02`.
