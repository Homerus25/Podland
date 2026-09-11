# 05: Migrate volume duplicate and image literals, run the full-suite gate

**What to build:** Two leftovers the architecture review card did not count, plus the closing gate. The volumes list-hook test re-declares a byte-identical copy of the volume factory: delete it and import the canonical one. The two image list tests re-declare whole image DTO literals in arrays although the image factory already exists: build those arrays from the shared factory, passing the asserted image id as an override if it differs from the canonical default. Assertions stay untouched. This ticket is also the closing batch of the epic: run the full frontend suite once (`npx vitest run`, must finish — a hang is a bug to diagnose, not a wait) and `npx biome check` on all changed files as the final gate.

**Blocked by:** None (both factories already exist; it can start immediately — though working it after 02–04 makes it the natural closing batch).

**Status:** ready-for-agent

- [ ] The volumes list-hook test imports the shared volume factory; its local copy is deleted
- [ ] The image list tests build their arrays from the shared image factory instead of DTO literals; no assertion is edited
- [ ] Full suite `npx vitest run` passes and completes
- [ ] `npx biome check` passes on all files touched by tickets 01–05
