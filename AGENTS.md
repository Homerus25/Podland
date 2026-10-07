# Agent Configuration for Podland

Podman Management Web-App. Issues are tracked on Forgejo, via the forgejo MCP tools — see `docs/agents/issue-tracker.md`.

`backend/go.mod`'s `replace go.podman.io/podman/v6` line is a local-environment tweak: it always shows dirty in `git status` — never stage or commit it.

## Navigation

- `docs/backend-conventions.md` - read before touching `backend/`: reference implementations to mirror per endpoint type (prune, streaming, permission gating), cross-domain duplication ruling, testing expectations.
- `docs/frontend-conventions.md` - read before touching `frontend/src/`: feature folder layout, React Query key scheme, component/test patterns, biome workflow.
- `GLOSSARY.md` - domain glossary and decision index (pointers into `docs/`).
- `docs/architecture-decisions.md` - system-level architecture decisions, §-numbered; code comments cite these numbers.
- `doku/` - API endpoint documentation (`description_*_api.md`); update the matching file when an endpoint's contract changes.
