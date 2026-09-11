# 03: Migrate quadlet tests to the shared factory

**What to build:** The quadlet feature's test files each re-declare a local quadlet fixture — seven copies, the largest pile in the suite. Delete the locals and import the shared factory from ticket 01. The create and edit tests use an `existing` id/name (with matching path and unit) instead of the canonical `web` default: those call sites pass their values as explicit overrides so their assertions keep passing unchanged. Assertions stay untouched.

**Blocked by:** 01 — Add container, quadlet, and secret fixture factories.

**Status:** ready-for-agent

- [ ] No local quadlet fixture remains in the quadlet feature's test files; every one imports the shared factory
- [ ] The create and edit tests pass their `existing` values as explicit overrides; no assertion is edited
- [ ] The quadlet feature's test files pass
