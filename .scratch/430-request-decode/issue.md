---
title: web.Decode owns the decode+validate ritual (architecture review C9)
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

The decode → validate → error ritual is hand-copied at 7 handler sites
(containers Create, images Pull, quadlets Create/Update/Generate/Import,
secrets Create) — past the conventions' own third-copy threshold — and
the copies speak three vocabularies: the seven mint
`ErrInvalidRequest.WithMessage("Invalid request body")` and then a fresh
`NewAppError("VALIDATION_FAILED", err.Error(), 400)` each; images Pull
writes the envelope itself before `streaming.Begin` with two
`slog.ErrorContext` 400 lines (the only client-error logging in the
codebase); machines Update/GrantAccess mint
`NewAppError("INVALID_REQUEST", err.Error(), 400)` — the raw JSON error
on the wire, no validator at all. Two decode-only stragglers inside the
envelope (volumes Create, and the machines pair) copy a fourth shape.
Bad-JSON handling is pinned at zero sites that own it once: nothing
stops the next handler from minting a fifth vocabulary.

## Solution

`web.Decode(r *http.Request, target any) *AppError` in `internal/web`
owns the ritual: JSON decode failure answers 400 `INVALID_REQUEST`
("Invalid request body"); struct validation failure answers 400
`VALIDATION_FAILED` with the validator's message (a new
`ErrValidationFailed` sentinel). All eleven call sites inside the
`web.Handle` subtree collapse to one decode line — returning the
AppError from the closure, or responding it directly pre-stream (images
Pull). No wire error code changes anywhere; doku documents codes only
and stays unchanged.

## Decisions (from the design session — do not relitigate)

1. **Scope — every decode site inside the envelope subtree (11)**: the
   seven ritual sites plus the decode-only stragglers volumes `Create`,
   machines `Update`, machines `GrantAccess` (validator is a no-op on
   their tagless DTOs; they stay tagless — their rules live in the
   service). Out of scope, documented as such: machines Create/List,
   users, auth — they don't cross `web.Handle`, and auth validates in
   the service by doctrine.
2. **One error vocabulary**: decode failure →
   `ErrInvalidRequest.WithMessage("Invalid request body")` everywhere —
   machines loses its raw `err.Error()` message (code unchanged, tests
   assert codes). Validation failure → a new `ErrValidationFailed`
   sentinel (code `VALIDATION_FAILED`, message = the validator's
   `err.Error()` via `WithMessage`), minted only by `web.Decode`.
3. **No client-error logging**: images Pull's two `slog.ErrorContext`
   400 lines are deleted — `web.Handle` doesn't log AppErrors, so
   400s-aren't-logged becomes universally true. Pull's info-level
   request log stays.
4. **One function, no pre-stream twin**: `web.Decode` returns
   `*AppError`; the pre-stream caller responds it itself with
   `errors.RespondWithError`. Pull is the only body-bearing
   `streaming.Begin` endpoint today (AutoUpdate reads a query param), so
   a second function waits for a second consumer — the repo's own
   extraction threshold.
5. **Secrets trim moves after Decode**: the validator runs on the
   untrimmed name; the handler then trims and applies its pattern and
   byte-cap checks. Whitespace-only names still answer
   `VALIDATION_FAILED` (the pattern check catches the trimmed empty
   string, same code). Accepted edge inversion: a name over 255 chars
   only because of surrounding whitespace now fails `max=255` (today it
   trims under the cap and passes) — the new behavior is the more
   defensible one.
6. **Validator instance**: one package-level shared validator instance
   in `internal/web` (thread-safe); no `DisallowUnknownFields` — no
   behavior change beyond decisions 2 and 5.
7. **Test split (pinned once)**: `web_test.go` pins `Decode` —
   malformed body and wrong-typed field (both 400 `INVALID_REQUEST`
   "Invalid request body"), missing-required field (400
   `VALIDATION_FAILED`), tagless struct and clean body (nil). The line
   between re-pin and contract: syntax/type decode errors are
   `Decode`'s, tag failures are the DTO's — so the per-domain
   malformed-body cases die (machines Update test, machines GrantAccess
   subtest, volumes Create ×2, secrets row, quadlets rows ×3) while
   every tag-failure row and the secrets whitespace-only row stay.
8. **doku unchanged**: doku documents error codes; no code changes on
   the wire. The machines message change is undocumented detail.

## Slices

- `430-request-decode-01-seam-and-migration` — `web.Decode` with its
  pinned suite, the `ErrValidationFailed` sentinel, all eleven call
  sites migrated, the re-pin tests deleted, backend green.
- `430-request-decode-02-conventions-update` — the conventions record
  the decode ruling (the fourth threshold crossing), `CONTEXT.md` gains
  the C9 entry, doku verified unchanged.
