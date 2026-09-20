# Podland

A full-stack web application for managing Podman on remote machines: a Go
backend drives Podman's Service API over SSH, a React frontend rides
React Query, SQLite stores machine metadata and users, and one container
serves the embedded UI and API on :8080.

## Documentation map

- `docs/architecture-decisions.md` — all system-level decisions, §-numbered
  (§1 podman integration … §16 machine registration). Code comments cite
  these numbers; keep them stable.
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
14. **Rate limiting** — fixed window; per-user 60/min, auth endpoints per-IP 10/min
15. **File handling** — no uploads; images pulled from registries only
16. **Machine registration** — dedicated endpoint, synchronous connection test

## Glossary

| Term | Definition |
|------|------------|
| **Machine** | A remote server with Podman installed that can be managed through this application |
| **Owner** | The user who registered a machine and provided its SSH credentials |
| **Shared User** | A user who has been granted access to a machine by its owner |
| **Connection Cache** | In-memory cache of SSH connections to remote machines |
| **Podman Service API** | The REST API provided by Podman for managing containers and images |
| **SSE** | Server-Sent Events, a standard for push notifications from server to client |
| **Endpoint envelope** | The uniform response contract every JSON endpoint speaks — failures as `{requestId, error{code, message}}`, successes as a DTO with an explicit status. _Avoid_: error wrapper, response helper |
| **Kit** | A shared module owning one ritual end to end (log-stream kit, stream transport, test kit); features and domains adapt to it, never copy it |

---

*Decision detail moved to `docs/` on 2026-09-20 to keep this file cheap to read; new settlements are dated inside the conventions docs (architecture-review candidates).*
