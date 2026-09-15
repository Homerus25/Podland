---
title: web.Decode, the ErrValidationFailed sentinel, and eleven call sites
parent: 430-request-decode
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - backend
  - web
  - testing
author: vibe-agent
---

## Parent

`.scratch/430-request-decode/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The endpoint envelope gains a decode twin and every body-bearing
handler inside the `web.Handle` subtree crosses it. `web.Decode(r,
&request) *AppError` in `internal/web` owns the decode+validate ritual
with one error vocabulary, and `web_test.go` pins it once. The seven
ritual sites (containers Create, images Pull, quadlets
Create/Update/Generate/Import, secrets Create) collapse to one decode
line inside their closures; the decode-only stragglers (volumes Create,
machines Update/GrantAccess) join them; images Pull responds the
AppError itself before `streaming.Begin`. For a user nothing changes:
the same 400 codes on the wire everywhere — machines' bad-JSON message
becomes the generic "Invalid request body", Pull stops logging its
400s, and bad-JSON handling gains exactly one owner.

## Acceptance criteria

- [ ] `internal/web` exports `Decode(r *http.Request, target any) *appErrors.AppError`: JSON decode failure returns `ErrInvalidRequest.WithMessage("Invalid request body")`; struct validation failure returns the `ErrValidationFailed` sentinel carrying the validator's message; nil on success. One package-level shared validator instance; no `DisallowUnknownFields`.
- [ ] `internal/errors` gains `ErrValidationFailed` beside `ErrInvalidRequest` (code `VALIDATION_FAILED`, status 400).
- [ ] `web_test.go` pins `Decode` once: malformed body → 400 `INVALID_REQUEST` "Invalid request body"; wrong-typed field → same; missing required field → 400 `VALIDATION_FAILED`; tagless struct → nil; clean body → nil.
- [ ] The seven ritual sites migrate: containers `Create`; quadlets `Create`, `Update`, `Generate`, `Import`; secrets `Create` (the name trim moves after `Decode`; the pattern and byte-cap checks stay handler-side); images `Pull`.
- [ ] images `Pull` decodes pre-stream — `if appErr := web.Decode(r, &request); appErr != nil { errors.RespondWithError(w, appErr, requestID); return }` — and its two `slog.ErrorContext` 400 lines are deleted; the info-level pull request log stays.
- [ ] The stragglers migrate: volumes `Create`, machines `Update`, machines `GrantAccess` — `INVALID_REQUEST` code unchanged, message now the generic one; their DTOs stay tagless (validation stays a no-op there).
- [ ] Re-pin tests die (syntax/type decode cases only): machines `TestMachineHandler_Update_MalformedBody`; machines GrantAccess "malformed body" subtest (the "invalid permission value" subtest stays); volumes `Create_MalformedBody` and `Create_MalformedJSON`; the secrets "malformed body" table row; quadlets' three "malformed JSON body" rows (the Update, Generate, and Import tables).
- [ ] Every tag-failure row and the secrets whitespace-only-name row stay and stay green (secrets' whitespace-only row may shift from validator's required-message to the pattern message — same `VALIDATION_FAILED` code).
- [ ] users, auth, and machines Create/List handlers are untouched.
- [ ] The backend suite is green and touched files pass `gofmt`.

## Blocked by

- None (can start immediately)

## Technical notes

- Do not touch conventions, `CONTEXT.md`, or doku in this slice; they ride in `430-request-decode-02`.
- Secrets edge (parent decision 5): a name longer than 255 only due to surrounding whitespace now fails `max=255` — accepted, more defensible than today's trim-under-the-cap pass.
