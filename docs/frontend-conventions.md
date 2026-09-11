# Frontend Conventions

How the React frontend (`frontend/src/`) is organized. Architecture decisions live in `CONTEXT.md`; API contracts in `doku/`. This file covers the patterns a new feature slice needs, so slices start from knowledge instead of re-reading sibling features.

Formatting is automated: a Crush pre-edit hook (`.crushrc` → `.crush/hooks/biome-format.sh`) runs Biome on a file before every edit, so files are already formatted when you read them. Run `npx biome check <changed files>` once at the end as a gate; never interleave `--write` runs with your own reads and edits.

## Feature folder layout

Each feature lives in `src/features/<name>/`:

- `services/<feature>Api.ts` — API client functions through the shared axios instance (`@shared/utils/api`), plus the **DTO response types colocated here** (see CONTEXT.md "DTO types"). Encode path segments with `encodeURIComponent`.
- `hooks/<feature>Api hooks` — React Query hooks in `use<Feature>.ts`.
- `components/` — UI components and their colocated tests.

## React Query

- Query keys come from the `shared/queries` constructors (`containersQueryKey`, `secretQueryKey`, `machineStatusQueryKey`, the `machinesQueryKey` constant, …); never build a `['machines', …]` array or a private key constructor in a feature. A new resource tab adds one `MachineResource` union member plus one plural list constructor in `queryKeys.ts`.
- List-invalidating mutations go through `useListInvalidatingMutation(machineId, resourceOrResources, run)` (`shared/queries`); cross-feature or streaming invalidation calls `invalidateMachineResources(queryClient, machineId, resourceOrResources)`. No optimistic updates (matches backend "list reflects server state" simplicity).
- Detail views that must always refetch use `gcTime: 0`. Values that must never be cached (revealed secrets) travel through `useMutation`, not `useQuery`.

## Components

- State display: loading / error / empty are **three distinct, data-testid'd states** (`<feature>-loading`, `<feature>-error`, `<feature>-empty`), each asserted in tests to exclude the other two.
- Errors render via `ErrorDisplay` with `getApiError(error)`; it accepts extra props (e.g. `data-testid`) and spreads them onto the alert.
- Tables use Chakra `Table`/`TableContainer` with `data-testid` on rows (`<item>-row`) so tests use `within(row)` for per-row assertions.
- Dates via `formatRelativeDate`, byte sizes via `formatBytes` (`@shared/utils/format`).
- Write affordances are permission-gated with helpers from `@features/machines/permissions`; below the needed level, show an explanatory hint, never disabled buttons (SettingsTab hide-and-hint pattern).

## Tests

- Vitest + Testing Library + MSW. Global handlers in `src/test/handlers.ts` cover only auth/health/status; **each test overrides its endpoints with `server.use(http.get('*/api/...', ...))`** using a `*Fixture()` helper building a typed object with `Partial` overrides. Shared fixture factories live in `src/test/factories.ts` — import from there instead of re-declaring a `*Fixture()` per test file; add a new factory there when a DTO is first tested.
- The full suite is a gate that must actually finish: it completes in under 2 minutes on a healthy tree (`npx vitest run`). During a change, run only the affected test files; run the full suite once at the end. A run that hangs or stretches past that is a bug to diagnose immediately, not a wait — a starved worker means an effect/async loop is blocking the event loop (see the LogViewer incident: unstable callback identities in a `useEffect` dep array).
- Component tests wrap the component in a fresh `QueryClient` with `retry: false` inside `QueryClientProvider`; hook tests use `renderHook` with the same wrapper.
- Test seams are the public interface (hook return value, rendered DOM), never internals.
- Query invalidation is asserted at the factory test (`useListInvalidatingMutation.test.tsx`, the sanctioned `vi.spyOn(queryClient, "invalidateQueries")`); feature tests assert refetch through MSW instead of invalidation spies. Streaming-hook tests keep their invalidation spies — the dry-run skip is bespoke logic nothing else covers.
- **Multipart/upload endpoints: MSW cannot consume FormData bodies in this environment** — any handler that reads the body (`request.formData()`, `request.text()`, …) hangs, and axios POSTs of FormData hang with it. Instead: extract the payload building into a pure function (e.g. `buildPopulateForm`) and unit-test that; in component tests, mock the single API function with `vi.mock` (partial mock keeping the rest of the module real) and note why. See `volumesApi.populate.test.ts` + `VolumeList.populate.test.tsx`.
- Cross-realm caveat when asserting on parsed multipart parts outside this limitation: MSW parses bodies with its own `File`/`Blob` implementations, so `instanceof File` fails — check for `"name" in part` instead.
- AbortSignal cross-realm caveat: under jsdom, `AbortController`/`AbortSignal` globals are jsdom's, and Node >= 24 (undici 7) rejects them in `fetch` (`instanceof` brand check), while DOM listeners (framer-motion) reject Node's — so neither global swap works. `src/test/setup.ts` installs a fetch relay after `server.listen` that passes a Node-realm signal to the fetch stack; don't create `fetch` calls with hand-built signals that bypass it, and don't swap the globals back.

## Machine detail page

Tabs in `pages/MachineDetailPage.tsx`: Containers, Volumes, Images, Quadlets, Secrets, Settings. A new resource tab goes in epic-specified position; read-only slices take only `machineId`, permission-gated slices also take `effectivePermission(machine)`.

DTO types and their field vocabularies (e.g. quadlet states: `unknown | stopped | running | error`; resources: container, pod, image, volume, network) are defined in each feature's `services/<feature>Api.ts` — read that file before writing tests or conditional UI against a resource's state.
