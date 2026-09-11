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

- Every table-backed resource list renders through `ResourceList` (`@shared/components/ResourceList`), the one owner of the list machinery: the loading/error/empty/table triad, the Chakra table chrome, the plural testid derivation (`{resource}s-loading`, `-error`, `-empty`, `-table`), the toolbar wrapper, and the permission hint. A feature list is an adapter: it keeps its query and mutation hooks and hands over resolved state — `resource` (plural noun), `isPending`, `error`, `items`, `columns` (data, not JSX: `{ label?, isNumeric? }[]`; a label-less entry renders the bare action-column header), `renderRow`, and optionally `toolbar`, `canManage`, `emptyMessage`. `SecretList` (`src/features/secrets/components/SecretList.tsx`) is the smallest reference adapter; a new resource tab copies it, not the machinery.
- The triad is **inline, never an early return**: the toolbar (or hint) renders above the state ternary, so toolbar and hint stay reachable while loading, on error, and when empty. Do not add a loading early-return to a list component.
- Errors render via `ErrorDisplay` with `getApiError(error)`; it accepts extra props (e.g. `data-testid`) and spreads them onto the alert. `ResourceList` uses it for the list-level error state; feature code keeps it for row- and dialog-level errors.
- Rows are feature-rendered: the Row component owns the singular `data-testid` (`<item>-row`) so tests use `within(row)` for per-row assertions. A clickable row keeps its `onClick` in the feature's row, and the action buttons inside such a row call `stopPropagation` so a button click does not also fire the row action.
- Dates via `formatRelativeDate`, byte sizes via `formatBytes` (`@shared/utils/format`).
- Hide-and-hint for resource lists is module-owned: pass `canManage` (computed with helpers from `@features/machines/permissions`); when false, `ResourceList` hides the toolbar and renders the canonical hint — "You have read-only access to this machine. Ask an owner or admin for write access to manage its {resource}s." — carrying `{resource}s-permission-hint` and `data-permission="read"`. A list without a permission concept omits `canManage`: toolbar always, no hint. Never render a feature-private variant of the list hint and never ship disabled buttons.

## Tests

- Vitest + Testing Library + MSW. Global handlers in `src/test/handlers.ts` cover only auth/health/status; **each test overrides its endpoints with `server.use(http.get('*/api/...', ...))`** using a `*Fixture()` helper building a typed object with `Partial` overrides. Shared fixture factories live in `src/test/factories.ts` — import from there instead of re-declaring a `*Fixture()` per test file; add a new factory there when a DTO is first tested.
- The full suite is a gate that must actually finish: it completes in under 2 minutes on a healthy tree (`npx vitest run`). During a change, run only the affected test files; run the full suite once at the end. A run that hangs or stretches past that is a bug to diagnose immediately, not a wait — a starved worker means an effect/async loop is blocking the event loop (see the LogViewer incident: unstable callback identities in a `useEffect` dep array).
- **List triad and hint behavior is pinned once, in `shared/components/ResourceList.test.tsx`**: mutual exclusion of the loading/error/empty/table states, plural testid derivation, the toolbar/hint swap, and the empty-copy override. A feature list suite keeps one loading smoke test (adapter wiring: the loading state renders with the toolbar still above it) plus all row/action/domain assertions; it consumes the derived testids as fixtures and does not re-pin exclusion.
- Component tests wrap the component in a fresh `QueryClient` with `retry: false` inside `QueryClientProvider`; hook tests use `renderHook` with the same wrapper.
- Test seams are the public interface (hook return value, rendered DOM), never internals.
- Query invalidation is asserted at the factory test (`useListInvalidatingMutation.test.tsx`, the sanctioned `vi.spyOn(queryClient, "invalidateQueries")`); feature tests assert refetch through MSW instead of invalidation spies. Streaming-hook tests keep their invalidation spies — the dry-run skip is bespoke logic nothing else covers.
- **Multipart/upload endpoints: MSW cannot consume FormData bodies in this environment** — any handler that reads the body (`request.formData()`, `request.text()`, …) hangs, and axios POSTs of FormData hang with it. Instead: extract the payload building into a pure function (e.g. `buildPopulateForm`) and unit-test that; in component tests, mock the single API function with `vi.mock` (partial mock keeping the rest of the module real) and note why. See `volumesApi.populate.test.ts` + `VolumeList.populate.test.tsx`.
- Cross-realm caveat when asserting on parsed multipart parts outside this limitation: MSW parses bodies with its own `File`/`Blob` implementations, so `instanceof File` fails — check for `"name" in part` instead.
- AbortSignal cross-realm caveat: under jsdom, `AbortController`/`AbortSignal` globals are jsdom's, and Node >= 24 (undici 7) rejects them in `fetch` (`instanceof` brand check), while DOM listeners (framer-motion) reject Node's — so neither global swap works. `src/test/setup.ts` installs a fetch relay after `server.listen` that passes a Node-realm signal to the fetch stack; don't create `fetch` calls with hand-built signals that bypass it, and don't swap the globals back.

## Machine detail page

Tabs in `pages/MachineDetailPage.tsx`: Containers, Volumes, Images, Quadlets, Secrets, Settings. A new resource tab goes in epic-specified position; read-only slices take only `machineId`, permission-gated slices also take `effectivePermission(machine)`. The tab's list component is a `ResourceList` adapter — copy the smallest existing one (`SecretList`), never hand-roll list machinery.

DTO types and their field vocabularies (e.g. quadlet states: `unknown | stopped | running | error`; resources: container, pod, image, volume, network) are defined in each feature's `services/<feature>Api.ts` — read that file before writing tests or conditional UI against a resource's state.

## Handoff: MutationDialog candidate (C6)

Dialog machinery — confirm dialogs, create modals, mutation reset-on-open/close invariants, error-code-to-hint mapping — is deliberately feature code today; `ResourceList` owns the read side only. Known gap that candidate must start from: VolumeList's row-triggered Delete/Populate dialogs call their state setters directly without `mutation.reset()` first (they only reset on close). The MutationDialog module must own reset-on-open so this class of gap disappears.

(Origin: candidate C6 of the 2026-09-09 architecture review; handoff recorded by ResourceList epic slice 6, also noted in `CONTEXT.md`.)
