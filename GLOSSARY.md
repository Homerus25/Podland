# Podland

A full-stack web application for managing Podman on remote machines: a Go
backend drives Podman's Service API over SSH, a React frontend rides
React Query, SQLite stores machine metadata and users, and one container
serves the embedded UI and API on :8080.

## Documentation map

- `docs/architecture-decisions.md` — all system-level decisions, §-numbered
  (§1 podman integration … §20 one progress-stream handler seam). Code
  comments cite these numbers; keep them stable.
- `docs/backend-conventions.md` — read before touching `backend/`:
  endpoint envelopes, streaming kit, connection seam, testing expectations.
- `docs/frontend-conventions.md` — read before touching `frontend/src/`:
  feature folders, React Query keys, component and test patterns.
- `doku/description_*_api.md` — API contracts per area; update the matching
  file when an endpoint's contract changes.
- `docs/agents/issue-tracker.md` — Forgejo tracker usage.

## Decisions at a glance

One line each; full detail in `docs/architecture-decisions.md`.

1. **Podman integration** — forked Podman, Service API over SSH tunnel, no CLI
2. **Remote machine access** — Podman pre-installed; everyone rides the owner's SSH connection
3. **Multi-user model** — owner-registered machines, shared via `read`/`write`/`admin` permissions; owner's key encrypted in SQLite
4. **Concurrency** — one cached connection per machine (dial dedup, LRU ≤ 100, real teardown); fail fast, no retries or queuing
5. **Auth & security** — JWT RS256 via Bearer header + revocable refresh sessions (httpOnly cookie, hashed, TTL'd)
6. **Real-time** — SSE for podman events; NDJSON HTTP streaming for progress and logs
7. **Error handling** — uniform endpoint envelope: failures `{requestId, error{code, message}}`
8. **Configuration** — environment variables only, no config files
9. **Theming** — Chakra UI + CSS variables, dark mode
10. **Offline** — detection via heartbeat + failure tracking, auto-reconnect
11. **Validation** — validator struct tags backend; form validation frontend
12. **Testing** — unit (fake engines), integration (podman test host), E2E (pod)
13. **Deployment** — one container: Node builds UI, Go embeds it, alpine runtime, :8080
14. **Rate limiting** — fixed window; per-user 60/min, auth endpoints per-IP 10/min, credential failures 5 per 15 min (login keyed per username, password change per caller, admin password reset per acting admin)
15. **File handling** — no uploads; images pulled from registries only
16. **Machine registration** — dedicated endpoint, synchronous connection test
17. **First-run bootstrap** — setup landing page creates the first admin (one-time endpoint); no baked-in credentials; container DB in the mounted `/app/data` volume
18. **Quadlet volume data migration** — opt-in `migrateVolumeData` on move/copy: preflight (409 non-empty target / 413 bound) → stop → export-import through the streaming kit; data before installs so a failure installs nothing
19. **One transfer kit behind every cross-machine copy/move** — `internal/transfer` owns the export→import splice, byte bound, emptiness probe, secret transfer, and the quadlet `Volume=`/`Secret=` reference resolution (one seam for the quadlet move and both move previews); quadlet move's data phase, the volumes copy/move, the secrets copy/move, and the images copy are its four compositions (§19)
20. **One progress-stream handler seam** — `streaming.HandleProgress` owns the handler-side ritual of every progress stream (operationID minting, the 10-minute write deadline, the operation slog line, the envelope, the run loop); domains pass an operation name, attrs, and their event vocabulary (§20)

## Glossary

| Term | Definition |
|------|------------|
| **Machine** | A remote server with Podman installed that can be managed through this application |
| **Owner** | The user who registered a machine and provided its SSH credentials |
| **Shared User** | A user who has been granted access to a machine by its owner |
| **Connection Cache** | In-memory cache of SSH connections to remote machines — one adapter of the Resource cache |
| **Resource cache** | A per-machine cache of one expensive resource (the podman Connection Cache, the SSH-exec client cache) whose lifecycle — acquire with dial dedup, refcounted release, LRU-idle eviction, idle cleanup, graceful removal, fail-fast invalidation — is one ritual (§4); an adapter hands over its dial and close, never re-implements the lifecycle |
| **Podman Service API** | The REST API provided by Podman for managing containers and images |
| **Network** | A named podman network (bridge/macvlan/host…) containers attach to; podman pre-creates a default bridge network; quadlets can define `.network` files |
| **SSE** | Server-Sent Events, a standard for push notifications from server to client |
| **Endpoint envelope** | The uniform response contract every JSON endpoint speaks — failures as `{requestId, error{code, message}}`, successes as a DTO with an explicit status. _Avoid_: error wrapper, response helper |
| **Progress stream** | An NDJSON endpoint whose response is a stream of progress events plus exactly one terminal event (image pull, quadlet auto-update, the move/copy transfers). The kit owns the handler-side ritual — operationID minting, the 10-minute write deadline, the operation slog line, the envelope, the run loop (`streaming.HandleProgress`, §20); domains adapt (operation name, attrs, event vocabulary), never re-assemble |
| **Kit** | A shared module owning one ritual end to end (log-stream kit, logs route kit, stream transport, transfer hooks, test kit); features and domains adapt to it, never copy it |
| **Companion quadlet** | A quadlet file on the same machine that another quadlet references by filename (a container's `Volume=app-data.volume`). Move/copy may carry the definition along — installed on the target before the referencing quadlet, skipped when the target's file is byte-identical; the volume's DATA follows only when the request opts into `migrateVolumeData` (§18) |
| **Volume data migration** | The move/copy transfer's opt-in data phase (§18): every volume-kind `Volume=` reference resolves (bare name, or the companion's `VolumeName=`, else `systemd-<name>`) and its content is exported from the source and imported into the target. Refuses non-empty target volumes (409) unless `overwriteVolumes` merges; answers as NDJSON progress events with one terminal event |
| **Secret transfer** | The move/copy transfer's opt-in secret phase (issue 215): every `Secret=` reference resolves (token before the first comma; `source=` governs the all-options form) and — with `transferSecrets` — its value is revealed on the source (admin on the source, the secrets API's reveal gate) and recreated on the target. Same-name skips before the reveal, secrets are copy-only, and per-reference outcomes ride the envelope's `secretTransfer` block |
| **Volume copy/move** | The volumes domain's cross-machine copy/move (issue 238, §19): a volume's content is exported from the source machine and imported into the target through the shared transfer kit. Always answers NDJSON progress events with one terminal event; refuses a same-name target volume (409 `VOLUME_ALREADY_EXISTS`) and an in-use source on a move (409 `VOLUME_IN_USE`, `force` overrides); a move removes the source volume after the data lands, a copy leaves it |
| **Secret copy/move** | The secrets domain's cross-machine copy/move (issue 239, §19): a secret's value is revealed on the source machine (admin — the reveal gate) and recreated on the target through the shared transfer kit. Answers one plain JSON envelope (no stream); refuses a same-name target secret (409 `SECRET_ALREADY_EXISTS`, re-checked at execution); a move removes the source secret after the target install, answering `partial` with the detail when the removal fails |
| **Image copy** | The images domain's cross-machine copy (issue 375, §19): an image's archive is exported from the source machine and loaded on the target through the shared transfer kit's image splice. Copy-only — no move mode, no force, no source removal; idempotent by construction (content-addressed, no rollback, no collision refusal); always answers NDJSON progress events with one terminal complete event carrying the image ID, byte count, loaded names, and both machine IDs; the bound is `IMAGE_TRANSFER_MAX_BYTES` (default 10 GiB, 413 `IMAGE_TRANSFER_TOO_LARGE` past it) |
| **Removal terminals** | The caller-owned vocabulary of a removal's two classified outcomes, behind `internal/removal`'s one entry point (issue 264): the vanished-source terminal — a plain value, nil-legal, every move source half's vanished-source-is-success — and the refusal terminal — a builder threading podman's refusal text where the domain's wire shows it — with the refusal predicate passed alongside, the domain's refusal phrases staying domain-local |
| **Transfer hooks** | The frontend kit behind every cross-machine move/copy surface (`shared/transfers`): the target-scoped preview query, the both-machines list invalidation (a partial result is success-shaped — the target install stands), the move-stream lifecycle over the stream transport, and the presentation module (`transferPresentation`) owning the phase vocabulary's progress labels and the stay-open report skeleton (the Moved/Copied header, the partial block with the pinned both-machines tail, the red error line) — the adapters stay the wording owners of their domain cause sentences and narrations (settled 2026-10-19, architecture-review candidate C8). A move surface (quadlet, volume, secret) adapts with its key constructor, fetcher, URL builder, event guard, progress fold, resource, and initial progress — never a copy; the image copy (issue 376) is the fourth adapter, copy-only — it pins the transfer dialog's mode (`pinnedMode`) instead of choosing it |
| **Connect ritual** | Attaching a running container to a network — one dialog owned by the networks feature, entered from either tab with the fixed side (the network from the Networks tab, the container from the Containers tab) and a picker for the other side; options that can only fail (stopped containers, already-attached pairs) never appear. The containers feature calls into networks for it, never the reverse |

---

*Decision detail moved to `docs/` on 2026-09-20 to keep this file cheap to read; new settlements are dated inside the conventions docs (architecture-review candidates).*
