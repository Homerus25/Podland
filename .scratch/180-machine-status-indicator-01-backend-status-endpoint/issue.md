---
title: "Backend: live machine status endpoint"
parent: 180-machine-status-indicator
date: 2026-08-29
status: ready-for-agent
labels:
  - ready-for-agent
  - machines
  - backend
  - feature
author: vibe-agent
---

## Parent

`.scratch/180-machine-status-indicator/issue.md`

## What to build

A live status check per machine, exposed as `GET /api/machines/{id}/status` behind the existing JWT and machine-access middleware (read level suffices). "online" means the SSH connection to the host succeeds AND the Podman service API answers its ping over that connection; anything else — dial failure, auth failure, missing socket, ping failure, or timeout — is "offline". There is no third server-side state.

The machines connection interface gains one new method: a live probe for an already-persisted machine that reuses the cached per-machine connection when present (a full SSH dial only when cold) and pings Podman through it, bounded by the same timeout regime as the registration connection test (10 seconds default, env-configurable). Any probe error — including timeout — yields offline.

Offline is a valid answer, not an error: the endpoint returns HTTP 200 with `{"status": "online" | "offline", "checkedAt": "<RFC3339 UTC>"}` whenever access is granted. 404 for an unknown machine, 403 for a user without access, matching the existing get-machine behavior. The detailed failure reason (host unreachable, auth rejected, socket missing) is logged server-side with the requestId and never appears in the response — machines are shared, and SSH-level errors describe the owner's infrastructure.

The list-machines endpoint is deliberately unchanged. Demoable via curl against a registered machine.

## Acceptance criteria

- [ ] `GET /api/machines/{id}/status` returns 200 `{"status":"online","checkedAt":...}` when SSH and the Podman ping succeed, reusing a cached connection when one exists
- [ ] Returns 200 `{"status":"offline","checkedAt":...}` on any probe failure, including timeout (10s default, env-configurable, same regime as the registration connection test)
- [ ] 404 for unknown machine, 403 for users without access; read-level access is sufficient
- [ ] `checkedAt` is RFC3339 UTC
- [ ] No connection error details in any response payload; probe failures logged server-side with the requestId
- [ ] Service unit tests at the mocked connection-manager seam: probe success → online, probe error → offline, probe timeout → offline, unknown machine → not found, missing access → denied
- [ ] Handler-level HTTP tests (first for the machines domain, following the secrets/quadlets handler-test pattern): 200 with payload for both states, 404, 403, read access accepted
- [ ] The machine API documentation gains this endpoint's contract

## Blocked by

None - can start immediately
