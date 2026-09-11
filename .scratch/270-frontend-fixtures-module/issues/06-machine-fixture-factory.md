# 06: Machine fixture factory for the page tests

**What to build:** The page-level tests re-declare the machine DTO by hand — five local fixture objects plus two array literals across the machines list and machine detail pages — because no machine factory exists in the shared fixtures module yet, although the conventions rule ("add a factory when a DTO is first tested") covers it. Add a `machineFixture` with `Partial` overrides to the module, canonical default being the shape the page tests already share (a production-web-style machine with admin permission), and migrate the locals to it. Per-test variety — permissions, sharing, editing scenarios — moves into explicit overrides; assertions stay untouched. This was excluded from the review card's scope by the 2026-09-08 grilling session and published here so it is not silently dropped.

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] The fixtures module exports a machine factory with the same `Partial`-override signature as the other five
- [ ] No local machine fixture object or literal array remains in the page tests; all build on the shared factory
- [ ] Per-test permission and sharing variety is expressed as overrides; no assertion is edited
- [ ] The machines list and machine detail page test files pass
