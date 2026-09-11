# 02: Migrate container tests to the shared factory

**What to build:** The container feature's test files each re-declare a local container fixture — five copies. Delete the locals and import the shared factory from ticket 01 instead. Two tests rely on defaults that differ from the canonical ones and must pass their values as explicit overrides so their assertions keep passing unchanged: the list test uses a different container id, and the create test uses an `existing`-style name. Assertions stay untouched — the same data renders. (Override policy settled in the 2026-09-08 grilling session: majority variant is the module default, minority call sites say what they depend on.)

**Blocked by:** 01 — Add container, quadlet, and secret fixture factories.

**Status:** ready-for-agent

- [ ] No local container fixture remains in the container feature's test files; every one imports the shared factory
- [ ] The two tests with divergent defaults pass them as explicit overrides; no assertion is edited
- [ ] The container feature's test files pass
