# 04: Migrate secret tests to the shared factory

**What to build:** The secrets feature's test files each re-declare a local secret fixture — four copies. Delete the locals and import the shared factory from ticket 01. Two tests diverge from the canonical defaults and pass their values as explicit overrides: the create test uses an `existing` id/name, and the detail test uses a different `updatedAt` timestamp. Assertions stay untouched.

**Blocked by:** 01 — Add container, quadlet, and secret fixture factories.

**Status:** ready-for-agent

- [ ] No local secret fixture remains in the secrets feature's test files; every one imports the shared factory
- [ ] The create test passes its `existing` values and the detail test its `updatedAt` as explicit overrides; no assertion is edited
- [ ] The secrets feature's test files pass
