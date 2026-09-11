---
title: "SSH exec streaming variant for remote command output"
parent: 190-quadlets-auto-update
date: 2026-08-30
status: ready-for-agent
labels:
  - ready-for-agent
  - quadlets
  - backend
  - infrastructure
  - ssh
author: vibe-agent
---

## Parent

`.scratch/190-quadlets-auto-update/issue.md` — Quadlet auto-update with live streaming output

## What to build

Extend the remote command execution seam (the interface the quadlet lifecycle uses for `systemctl`, with its per-machine cached SSH clients and fail-fast discipline) with a streaming variant. Same inputs as the existing synchronous exec — context, machine id, user id, command, options — plus a callback that receives each output line as it arrives, tagged with which remote stream it came from (stdout or stderr).

The call still returns the full captured result (captured output and exit code), so callers can parse a final machine-readable report from stdout and interpret exit codes exactly as they do today. The real implementation tees the SSH session's output into the callback while capturing; lines must be emitted as they arrive, not batched after completion.

Seam semantics are unchanged: a non-zero exit code is a result, not an error (the caller interprets it); SSH dial/session failures are errors; context cancellation aborts the session and returns an error. The shared test mock gains the method with call recording in the existing style — recorded command, options, and emitted lines available to assertions, with configurable result/error.

This slice is infrastructure only (same shape as the original SSH-exec slice under the quadlets epic): no HTTP endpoint is added. The auto-update endpoint slice consumes it.

## Acceptance criteria

- [ ] Streaming variant on the SSH exec interface: line callback with stream tag, returns captured result (output + exit code)
- [ ] Lines are emitted as they arrive from the SSH session, not after the command completes
- [ ] Non-zero exit code still returned as a result, not an error; SSH dial/session failure is an error; context cancellation aborts the session and errors
- [ ] Per-machine SSH client caching, invalidation on failure, and fail-fast (no retry) behavior unchanged
- [ ] The shared mock records streaming calls (command, options, emitted lines) and supports configured results/errors, matching the existing mock recording style
- [ ] Unit tests cover: success with interleaved stdout/stderr lines in arrival order, non-zero exit code, connection failure, cancellation mid-stream
- [ ] `go build ./...` succeeds, `go vet ./...` is clean, existing tests pass

## Blocked by

None - can start immediately
