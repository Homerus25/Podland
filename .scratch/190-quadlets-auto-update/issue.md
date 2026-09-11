---
title: Quadlet auto-update with live streaming output
parent: none
date: 2026-08-30
status: ready-for-agent
labels:
  - ready-for-agent
  - quadlets
  - backend
  - frontend
  - feature
author: vibe-agent
---

## Problem Statement

Keeping quadlet-managed containers up to date currently requires the user to SSH into each machine by hand and run `podman auto-update`. There is no way to see, from the web app, which containers have newer images available, and no way to trigger the update without leaving the UI. Long-running updates also produce a wall of terminal output that the user cannot observe from the app, so even after SSHing in they are effectively updating blind.

## Solution

The quadlets panel on the machine detail page gains two actions: "Check for updates", which runs `podman auto-update --dry-run` and reports which containers have pending image updates, and "Update all", which runs the real `podman auto-update` after a confirmation. Both are backed by a single streaming endpoint that runs the command on the machine over SSH and forwards its output live to the browser as NDJSON events, ending with a parsed per-container report (updated, failed, or pending). Native podman semantics apply: only containers whose quadlet declares an auto-update policy participate; containers without the policy are untouched.

## User Stories

1. As a machine owner, I want to trigger `podman auto-update` for a machine's quadlets from the web app, so that I don't have to SSH in and run it manually.
2. As a machine owner, I want the remote command's output streamed live into the app while it runs, so that I can watch progress instead of staring at a spinner.
3. As a machine owner, I want a parsed per-container report when the run finishes (which updated, which failed), so that I know the outcome without reading raw logs.
4. As a machine owner, I want a dry-run "check for updates" that changes nothing, so that I can see what would update before committing.
5. As a machine owner, I want a confirmation dialog before the real update fires, so that I don't accidentally restart production containers.
6. As a machine owner, I want partial failures reported per container while successful updates are still applied, so that one broken container doesn't hide the rest of the outcome.
7. As a machine owner, I want a clear connection error when the machine is unreachable over SSH, so that I can distinguish "host is down" from "update failed".
8. As a shared user with write access, I want to run auto-update on machines shared with me, so that I can maintain them without being the owner.
9. As a read-only shared user, I want the auto-update action unavailable and rejected by the API, so that visitors cannot restart my containers.
10. As a user without access to a machine, I want requests for it rejected with the same 404/403 behavior as the rest of the machine subtree, so that machine existence is not leaked.
11. As a user, I want the quadlet, container, and image lists invalidated after an update completes, so that the UI reflects restarted containers and new images without a manual reload.
12. As a user, I want long-running updates to keep streaming past the server's usual 30-second response window, so that multi-image pulls aren't cut off mid-run.
13. As a user, I want the run aborted when I disconnect, so that behavior is predictable and matches how image pulls behave.
14. As a user, I want an in-progress indicator while the stream is open, so that I know the run hasn't stalled.
15. As a user, I want the dry-run report to list containers with pending updates and their images, so that I can decide whether to run the real update.
16. As a user, I want dry-run and real-run clearly distinguished in the UI, so that I always know whether I'm mutating.
17. As an admin, I want auto-update runs logged server-side with the requestId and user, so that I can trace who updated what and when.
18. As a developer, I want the remote command executed through the existing SSH exec seam, so that the feature is mockable exactly like the quadlet lifecycle operations.
19. As a developer, I want the streaming response to follow the same NDJSON event conventions as image pulls, so that the frontend consumes both uniformly.
20. As a developer, I want the endpoint contract documented in the quadlet API documentation, so that the API docs stay the source of truth.

## Implementation Decisions

Rulings from the grilling session on 2026-08-30:

- **Native auto-update semantics.** The feature runs `podman auto-update` as-is. Only containers with the auto-update policy label (quadlets declaring `AutoUpdate=registry`) are considered; there is deliberately no force-update mechanism for quadlets without the policy, and no extra surfacing of skipped containers. The original ask ("update all containers related to quadlets") was narrowed to podman's native contract during grilling.
- **Domain ownership.** The quadlets domain owns the operation, since it is an operation on a machine's quadlet collection and that service already holds the remote-command capability. Route: `POST /api/machines/{machineId}/quadlets/auto-update` with optional `dryRun=true` query parameter, gated behind the existing JWT, machine-access, and write-permission middleware.
- **Execution mechanism.** The command runs over the raw SSH command-execution path (the same seam the quadlet lifecycle uses for `systemctl`), not the Podman service API: the vendored Podman bindings expose no auto-update API at all. Command assembled as `podman auto-update --format json` plus `--dry-run` when requested. `--rollback` stays at its CLI default and is not exposed; authfile and TLS-verify options are not exposed in v1.
- **Streaming seam extension.** The existing SSH exec interface gains one callback-based streaming variant: same inputs as the synchronous exec, plus a callback receiving output lines as they arrive; the captured result (full output, exit code) is still returned so the caller can parse the final report and interpret the exit code. This is an extension of the proven mockable seam, not a new seam.
- **Event protocol.** NDJSON mirroring the image-pull streaming events: log events carry the operation id, machine id, and one remote output line; a terminal complete event carries the parsed per-container report; an error event is reserved for failures to run the command at all (SSH dial failure, session failure). Per-container update failures are data inside the complete report, never an error event. If the trailing JSON report cannot be parsed, the stream still completes: log events have already delivered the raw output, and the complete event reports that the report was unparsable rather than failing the whole stream.
- **Long-run deadline handling.** The streaming handler extends the per-response write deadline (now plus 10 minutes) via the standard library's response controller. The server's global 30-second write timeout — which protects every other endpoint — is deliberately left untouched.
- **Disconnect behavior.** Client disconnect cancels the run, mirroring the image-pull behavior: the request context tears down the SSH session and the command aborts.
- **Frontend UX.** The quadlets panel toolbar on the machine detail page gains "Check for updates" (dry-run, no confirmation) and "Update all" (confirmation modal). Both open a streaming progress modal mirroring the image-pull progress UI. On completion, the quadlet, container, and image queries are invalidated. The machines overview cards stay lean — no auto-update action there (explicitly rejected during grilling).
- **Documentation.** The quadlet API documentation gains the endpoint's contract, including the event sequence and report shape.

## Testing Decisions

Good tests assert externally visible behavior only: HTTP status codes, NDJSON event payloads and ordering, the parsed report contents, and rendered UI. Never which internal path produced them.

- **Service tests at the extended SSH exec mock seam**: emitted log events in order, dry-run flag propagated into the command, report parsed from the captured output, partial-failure exit codes yield a complete report with per-container errors, command-execution failures map to connection errors, unknown machine and missing access map to the standard errors. The mock gains the streaming method with call recording, in the same style as its existing synchronous recording. Prior art: the quadlet lifecycle service tests over the same mock.
- **Handler tests at the established router-mirror seam**, parsing NDJSON response bodies: successful stream with terminal complete event, partial failures still completing, connection failure producing the error event envelope, 404/403 permission behavior, and dry-run parameter handling. Prior art: the image-pull streaming handler tests and the quadlet lifecycle handler tests.
- **Integration tests** against the existing Podman harness focus on the dry-run mode (deterministic, no real registry pulls): endpoint contract, permission gating, and event sequence with a policy-labeled quadlet fixture. Real update runs against live registries are too flaky for the harness and stay uncovered.
- **Frontend tests** follow the existing quadlet feature hook/component patterns: stream consumption, modal states, confirmation gating of the real run, and query invalidation on completion.

## Out of Scope

- Force-updating quadlets that lack an auto-update policy (the broader "update everything" mechanism, explicitly rejected).
- Surfacing which quadlets were skipped for lacking the policy (the "native + visibility" option, explicitly rejected).
- Exposing rollback, authfile, or TLS-verification options.
- Scheduled or recurring auto-updates.
- An async job model with resumable runs (explicitly rejected in favor of streaming).
- Any machine-wide maintenance operations beyond quadlet auto-update.
- Raising or making the global server write timeout configurable.
- Fixing the pre-existing 30-second write-timeout ceiling that also affects image-pull streaming (discovered during design; separate issue if desired).

## Further Notes

- The Podman v6 Go bindings expose no auto-update API, which is why raw SSH command execution is the only mechanism; this was verified against the vendored library, not assumed.
- `podman auto-update --format json` emits the human-readable progress on one stream and the machine-readable report as the final output, which is what makes both live streaming and a parsed terminal report possible from a single run.
- The existing image-pull streaming endpoints share the 30-second global write-timeout ceiling; this feature's per-response deadline extension is the established pattern any such fix would follow.
