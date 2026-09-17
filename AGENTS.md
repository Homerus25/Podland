# Agent Configuration for Podland

Podman Management Web-App. Issues are tracked on Forgejo, via the forgejo MCP tools — see `docs/agents/issue-tracker.md`.

## Navigation

- `docs/backend-conventions.md` - read before touching `backend/`: reference implementations to mirror per endpoint type (prune, streaming, permission gating), cross-domain duplication ruling, testing expectations.
- `docs/frontend-conventions.md` - read before touching `frontend/src/`: feature folder layout, React Query key scheme, component/test patterns, biome workflow.
- `CONTEXT.md` - domain model and architecture decisions.
- `doku/` - API endpoint documentation (`description_*_api.md`); update the matching file when an endpoint's contract changes.
