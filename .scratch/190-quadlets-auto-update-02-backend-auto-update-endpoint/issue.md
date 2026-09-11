---
title: "Backend: streaming quadlet auto-update endpoint"
parent: 190-quadlets-auto-update
date: 2026-08-30
status: ready-for-agent
labels:
  - ready-for-agent
  - quadlets
  - backend
  - feature
  - streaming
author: vibe-agent
---

## Parent

`.scratch/190-quadlets-auto-update/issue.md` — Quadlet auto-update with live streaming output

## What to build

`POST /api/machines/{machineId}/quadlets/auto-update` with optional `dryRun=true` query parameter, mounted in the quadlet subtree behind the existing JWT, machine-access, and write-permission middleware. The service assembles `podman auto-update --format json` (plus `--dry-run` when requested) and runs it on the machine through the streaming SSH exec seam.

The handler responds as an NDJSON stream using the reusable streaming loop from the prefactor slice:

- **Log events** as remote output lines arrive — operation id, machine id, one line per event.
- **Terminal complete event** carrying the parsed per-container report: container name, image, per-container outcome (updated / failed with error; for dry runs, pending-update indicator), plus an overall status. Per-container update failures are data in this report even when the command exits non-zero — a partially failed run still completes, never errors.
- **Error event** only when the command could not run at all (SSH dial/session failure) or the run was cancelled by client disconnect.

If the trailing JSON report on stdout cannot be parsed, the stream still completes: the raw output has already been delivered via log events, and the complete event reports an explicit unparsable-report status rather than failing the stream.

Because registry pulls make runs routinely exceed the server's 30-second global write timeout, the handler extends the per-response write deadline (now + 10 minutes) via the standard library's response controller. The global write timeout — which protects every other endpoint — stays untouched. Client disconnect cancels the request context, which tears down the SSH session and aborts the run, mirroring image-pull behavior. Runs are logged server-side with the requestId and user.

Demoable via curl against a registered machine: watch log lines arrive, then the terminal report.

## Acceptance criteria

- [ ] Route registered under the quadlet subtree; write permission required (read-level 403), unknown machine 404, no-access 404/403 consistent with the rest of the subtree
- [ ] `dryRun=true` appends `--dry-run` to the assembled command (asserted at the mocked seam); without it a real update runs
- [ ] NDJSON sequence: zero or more log events, then exactly one terminal event (complete or error)
- [ ] Complete event carries per-container report entries (name, image, outcome, error detail); partial failures yield a complete event with failed entries, not an error event
- [ ] Non-zero exit with a valid report still yields a complete event; unparsable trailing JSON yields a complete event with an explicit unparsable-report status
- [ ] SSH connection failure yields the standard error event envelope; client disconnect cancels the remote run
- [ ] Per-response write deadline extended to now + 10 minutes inside the handler; the global server write timeout is not modified
- [ ] Runs logged server-side with requestId and user
- [ ] Service tests at the mocked streaming seam: command assembly and dry-run propagation, event emission order, report parsing, exit-code semantics, connection-error mapping
- [ ] Handler tests parse NDJSON response bodies (success stream, partial failure, connection error, 403/404, dry-run param), following the image-pull streaming handler tests as prior art
- [ ] `go build ./...` succeeds, `go vet ./...` is clean, existing tests pass

## Blocked by

- 190-quadlets-auto-update-00-prefactor-streaming-loop (reusable streaming loop)
- 190-quadlets-auto-update-01-ssh-exec-streaming (streaming seam the service calls)
