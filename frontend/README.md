# Podland Frontend

Vite + React 18 + TypeScript (strict) app for managing Podman on remote machines.

## Scripts

| Script | What it does |
| --- | --- |
| `npm run dev` | Dev server on port 3000, proxying `/api` to the backend on 8080 |
| `npm run build` | Typecheck both tsconfig projects, then production build to `dist/` |
| `npm run preview` | Serve the production build locally |
| `npm test` | Run the Vitest suite once |
| `npm run test:watch` | Vitest in watch mode |
| `npm run typecheck` | `tsc --noEmit` for `tsconfig.json` (src) and `tsconfig.node.json` (vite config) |

## Layout

Feature-sliced structure with path aliases (`@` → `src`, `@features` → `src/features`, `@shared` → `src/shared`), configured in `vite.config.ts` and `tsconfig.json` so they resolve in dev, build, editor, and tests:

```
src/
├── features/   # Feature modules (auth, machines, ...) — filled in by later slices
├── pages/      # Route-level pages
├── shared/     # Cross-feature code (API client, types, hooks, components)
└── test/       # Test harness: MSW server, handlers, global setup
```

## Test seam

Tests standardize on a single network-level seam: MSW intercepts HTTP requests, so the real axios client (`src/shared/utils/api.ts`), interceptors, and React components are exercised end to end. Handlers in `src/test/handlers.ts` mirror the real backend API shapes; unhandled requests fail tests.

## Environment

See `.env.example`. `VITE_API_BASE_URL` (default `/api`), `VITE_APP_NAME` (default `Podland`), `VITE_DEFAULT_THEME` (`light`/`dark`/`system`, default `system`).
