# 01: Add container, quadlet, and secret fixture factories

**What to build:** The shared test fixtures module — the single source the frontend conventions bless for DTO shapes — currently defines factories only for the volume and image DTOs. Add the three missing list-DTO factories: container, quadlet, and secret. Each takes `Partial` overrides and returns a fully valid DTO, matching the signature and spirit of the existing two (defaults distinctive enough to assert against without being tautological). This is the expand step of an expand–contract: nothing imports the new factories yet; the migrations are tickets 02–04. Scope and canonical defaults were settled in a grilling session on 2026-09-08.

Canonical defaults are the majority variants already in use across the test suite, so most call sites need no overrides:

- Container: id `c-1`, names `["/my-container"]`; all other fields the values shared by the existing local copies.
- Quadlet: id/name `web` with matching `web.container` path and `web.service` unit; all other fields the shared values.
- Secret: id `3f2e1d0c`, name `db-password`; all other fields the shared values.

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] The fixtures module exports container, quadlet, and secret factories with the same `Partial`-override signature as the existing volume and image factories
- [ ] Canonical defaults are the majority variants listed above
- [ ] No existing test file is modified and no test import is added yet
- [ ] `npx biome check` passes on the module
