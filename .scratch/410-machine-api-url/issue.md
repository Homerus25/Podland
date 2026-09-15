---
title: One owner for machine-scoped URLs (architecture review C11)
parent: none
date: 2026-09-15
status: open
labels:
  - needs-triage
  - frontend
  - refactoring
author: vibe-agent
---

## Problem Statement

Machine-scoped URL construction has no owner. Three encoding conventions
spread across the service layer: volumesApi encodes machineId and the
resource segment, containers/quadlets/secrets encode only the resource,
machinesApi encodes nothing, and imagesApi encodes the machineId on one
line and not the next. Two hooks (`usePullImage`,
`useQuadletAutoUpdate`) re-derive full request URLs — base prefix and
all — that the services layer owns, and the volume export button in
`VolumeList` hardcodes `/api` into its download URL, so a configured
`VITE_API_BASE_URL` breaks volume export while everything else works.
The design session sweep also found `secretsApi` building machine-scoped
URLs four times, which the review's file list missed — the drift is one
feature wider than reported.

## Solution

One builder pair in `shared/utils/api` owns machine-scoped URL
construction: `machineApiPath` emits the path (every segment encoded),
`machineApiUrl` prepends the configured base. Every service routes its
machine-scoped literals through them — axios calls take the path form,
fetch consumers (log streams, downloads) the full form. The services
that streams and downloads belong to export their URL builders next to
their request functions, so hooks and the export button stop knowing
paths or the base entirely. The hardcoded `/api` dies, one encoding
rule survives, and `apiBaseUrl` goes module-private so the compiler
enforces the single owner.

## Decisions (from the design session — do not relitigate)

1. **Builder shape**: two exports in `shared/utils/api`.
   `machineApiPath(machineId, ...segments)` is the primitive —
   `/machines/{enc}` with every segment `encodeURIComponent`-encoded.
   `machineApiUrl(machineId, ...segments)` is `apiBaseUrl` + that path,
   with the join stripping a trailing `/` from the base so a
   `VITE_API_BASE_URL` like `https://host/api/` cannot produce `//`.
   Axios call sites take the path form (the shared instance already
   prepends the base and would double it otherwise); fetch consumers —
   the stream transport and `downloadFile` — take the full form.
2. **Encoding rule**: every segment is encoded, machineId first among
   them. This is what the frontend conventions already mandate and what
   volumesApi already proves on the wire; for UUID-style machine IDs
   the requests are byte-identical, and the backend router decodes
   percent-encoding transparently.
3. **Scope**: all machine-scoped URL construction in all six services —
   machines, containers, quadlets, images, volumes, and secrets (the
   last missed by the review) — migrates to the builders. App-router
   navigations (`navigate(\`/machines/...\`)`) are page routes, not API
   URLs, and stay. The `streamTransport` interface is untouched.
4. **Single-owner enforcement**: `apiBaseUrl` becomes module-private.
   Nothing outside `shared/utils/api` can re-derive a full URL, which
   is exactly the drift C11 found twice. The configured-base test
   stubs `import.meta.env` and dynamically re-imports the module rather
   than needing the export back.
5. **Service-level URL exports**: `imagesApi.pullStreamUrl(machineId)`,
   `quadletsApi.autoUpdateStreamUrl(machineId, { dryRun })` (the
   `?dryRun=true` query is the builder's, not the caller's), and
   `volumesApi.volumeExportUrl(machineId, volumeName)` sit beside their
   request functions. Log-stream URLs are not exported — their only
   consumers are `followContainerLogs`/`followQuadletLogs`, which build
   them via `machineApiUrl` privately with `?tail=&follow=true` inline
   (single occurrence each); the private `containerUrl`/`quadletUrl`
   helpers are deleted.
6. **Testing**: unit tests pin the builders — per-segment encoding
   including machineId, base prefix, trailing-slash-safe join — and one
   MSW-level test pins the configured-base class (env stubbed to a
   non-default origin, a stream/download fetch observed hitting it).
   The existing MSW suites are the regression net for the retrofit and
   are expected to pass unchanged.

## Slices

- `410-machine-api-url-01-builder-and-retrofit` — the builder pair plus
  module-private `apiBaseUrl`, the six-service retrofit, the three URL
  exports feeding both hooks and the export button, builders pinned by
  unit tests plus the configured-base test; the hardcoded `/api` bug
  dies.
- `410-machine-api-url-02-conventions-update` — the frontend
  conventions name the builder as the owner of machine-scoped request
  URLs, and `CONTEXT.md` gains the C11 decision entry.
