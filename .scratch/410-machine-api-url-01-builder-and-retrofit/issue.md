---
title: machineApiPath/machineApiUrl and the six-service retrofit
parent: 410-machine-api-url
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - refactoring
  - testing
author: vibe-agent
---

## Parent

`.scratch/410-machine-api-url/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

Machine-scoped URLs get their single owner and everyone moves onto it.
`shared/utils/api` grows the `machineApiPath`/`machineApiUrl` pair and
stops exporting `apiBaseUrl`. All six services — machines, containers,
quadlets, images, volumes, secrets — route every machine-scoped path
through the builders, killing the private `containerUrl`/`quadletUrl`
helpers and the per-file encoding drift. The image-pull and
quadlet-auto-update hooks take their stream URLs from their services'
new exports instead of re-deriving paths, and the volume export button
takes its download URL from `volumesApi` — the hardcoded `/api` dies,
so a configured `VITE_API_BASE_URL` no longer breaks volume export
while the rest of the app honors it. For a user on the default base
nothing changes; for a user on a configured base, volume export starts
working.

## Acceptance criteria

- [ ] `shared/utils/api` exports `machineApiPath(machineId: string, ...segments: string[])` and `machineApiUrl(machineId: string, ...segments: string[])`; every segment — machineId first — passes through `encodeURIComponent`; `machineApiUrl` prefixes the base with a join that strips a trailing `/` from it
- [ ] `apiBaseUrl` is no longer exported from `shared/utils/api` (the axios instance consumes it internally); no file outside that module imports it
- [ ] No hand-built machine-scoped URL literal remains in any of the six services: axios calls take the path form, the log-stream `follow*` functions take the full form with `?tail=&follow=true` appended inline; `containerUrl` and `quadletUrl` are deleted
- [ ] `imagesApi` exports `pullStreamUrl(machineId)`; `usePullImage` passes it to the stream transport and no longer imports `apiBaseUrl` or knows the path
- [ ] `quadletsApi` exports `autoUpdateStreamUrl(machineId, { dryRun })` with the `?dryRun=true` query owned by the builder; `useQuadletAutoUpdate` consumes it and drops its `URLSearchParams` ritual and `apiBaseUrl` import
- [ ] `volumesApi` exports `volumeExportUrl(machineId, volumeName)`; the volume export button consumes it — no hardcoded `/api` remains anywhere in `features/`
- [ ] Unit tests pin the builders: per-segment encoding including machineId, base prefix, trailing-slash-safe join
- [ ] One MSW-level test pins the configured-base class: with `VITE_API_BASE_URL` stubbed to a non-default origin (env stub + dynamic module re-import), a stream or download fetch is observed hitting the configured base
- [ ] `npx vitest run` is green with the existing MSW suites unchanged (they are the regression net; handler fixture IDs that encode to themselves must keep matching)
- [ ] Touched files pass `npx biome check`

## Blocked by

- None (can start immediately)

## Technical notes

- MSW handlers match on the path; UUID-style fixture IDs encode to themselves, so suites pass unchanged. If a fixture ever contains a character that encodes differently, move the handler to the encoded form — never special-case the builder.
- `streamTransport` and `downloadFile` signatures are untouched; only the URLs fed into them change ownership.
- Do not touch the conventions docs or `CONTEXT.md` here; they ride in `410-machine-api-url-02`.
