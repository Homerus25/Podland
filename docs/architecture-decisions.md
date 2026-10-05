# Architecture Decisions

System-level decisions for Podland, moved out of `CONTEXT.md` (2026-09-20)
so that file stays a lean glossary and index. The § numbers below are
stable — code comments cite them (`docs/architecture-decisions.md §4`).

Kit-level settlements — which shared module owns which ritual — are
recorded inline in `docs/backend-conventions.md` and
`docs/frontend-conventions.md`, each dated with its architecture-review
candidate number. API contracts live in `doku/`.

## Core Architecture Decisions

### 1. Podman Integration
- **Forked Podman** extended for custom needs (Project folder: ~/repos/podman/)
- **Backend interacts via Podman Service API** (REST over Unix socket, accessed via SSH tunnel)
- **Connection Mechanism**: Uses `~/repos/podman/pkg/bindings/connection.go` for SSH tunneling
- **No CLI integration** - only Go API usage via the connection package

### 2. Remote Machine Access
- Podman must be **pre-installed** on remote machines
- **No deployment of Podman** - backend assumes it exists
- Backend uses owner's SSH credentials to connect to remote machines
- All users access a machine through the **owner's SSH connection**

### 3. Multi-User Model
- Each machine is **registered by an owner** (user)
- Owner's SSH private key is stored **encrypted in SQLite**
- Machine can be **shared with other users** via permission table
- Application-level permissions enforced: `read`, `write`, `admin`
- **One connection per machine** (not per user) - cached and reused

### 4. Concurrency & Performance
- **One connection per machine**: Each machine has at most one active SSH connection, cached and reused
- **Per-machine in-flight dial dedup**: concurrent cold acquisitions for the same machine share a single dial; dials to different machines never serialize behind a lock
- **Connection lifetime is the connection's, not the request's**: the cached connection context is detached from the request that dialed it, so an HTTP handler returning never kills the connection for later cache hits
- **Engine handover at the connection seam**: each domain (volumes, secrets, containers, images, quadlets) declares a caller-width engine interface and a one-method provider that wraps `connections.Manager.WithConnection`; the production adapter constructs the podman engine inside the connection lease, so no domain service builds one inline
- **Global limit**: Maximum cached machine connections = 100 (configurable, `MAX_CONCURRENT_MACHINES`)
- **LRU eviction**: When the cache is full, the least recently used idle connection is closed
- **Real teardown**: LRU eviction, the idle-timeout cleanup (15 minutes), and machine removal genuinely close the SSH session; removal is graceful (in-flight calls finish first)
- **Fail fast** on connection failures/timeouts (no retry logic, no request queue)
- **React Query** for frontend state management and caching

### 5. Authentication & Security
- **JWT tokens** (RS256 asymmetric keys)
  - Transported via **Authorization: Bearer** header
  - Frontend redirects to login page on expiration (401 interceptor)
- **Refresh sessions** (revocable, server-side)
  - Login additionally sets an **httpOnly refresh cookie**: opaque 256-bit token, `Secure`, `SameSite=Strict`, scoped to `/api/auth`
  - Only the token's **SHA-256 hash** is persisted (sessions table, one row per login, multi-device)
  - TTL via `SESSION_TTL_DAYS` (default 30); login lazily purges the user's expired rows
- **Key Storage**: File-based (PEM files) with auto-generation on startup
- **CORS**: Environment variables for allowed origins
- **Sensitive data encryption**: `github.com/secure-systems-lab/go-securesystemslib`
  - Encrypts SSH private keys and other secrets before SQLite storage

### 6. Real-Time Updates
- **Server-Sent Events (SSE)** for real-time Podman events
  - Container start/stop, image pulls, etc.
- **Centralized event bus** in frontend for distributing SSE events
- **Lazy connections**: Connect on first subscriber, disconnect on last unsubscribe
- **Abortable operations** with progress tracking via HTTP streaming

### 7. Error Handling
- **Endpoint envelope**: the uniform response contract every JSON endpoint speaks — machine-scoped routes cross `web.Handle`, user-scoped routes outside the machine subtree cross its twin `web.HandleUser` (context user ID passed through as-is, empty is a legal state) — failures as `{requestId, error{code, message}}`, successes as a DTO with an explicit status. Full ruling and reference implementations: `docs/backend-conventions.md`. _Avoid_: error wrapper, response helper
- **Frontend**:
  - Detailed error messages for client errors (4xx)
  - Generic error messages for server errors (5xx)
  - Context-based error display
  - Global 401 interceptor for token expiration
- **Backend**:
  - Logs via slog
  - Fail fast on SSH/Podman failures
  - Structured error responses with requestId

### 8. Configuration
- **Go backend**: Environment variables only
- **Frontend**: Environment variables via Vite
- **No config files** for sensitive data

### 9. Theming
- **Chakra UI** for component library and theming
- **CSS variables** with theme provider
- Supports **dark mode** (system preference + user override)

### 10. Offline Handling
- Redirect to offline page when connection to backend is lost
- **Detection**: Combination of `navigator.onLine`, heartbeat endpoint, and request failure tracking
- **Auto-reconnect**: Automatically attempt to reconnect, redirect back when online

### 11. Input Validation
- **Go backend**: Struct tags + `github.com/go-playground/validator`
- **Frontend**: Form validation with React Hook Form or similar

### 12. Testing Strategy
- **Unit tests**: Mocked Podman responses
- **Integration tests**: Real Podman instance in container (orchestrated via Podman pods)
- **E2E tests**: Frontend + backend + Podman in Podman pod

### 13. Deployment & Distribution
- **One container image**: a Node stage builds the frontend, a Go stage embeds it into the backend binary, the runtime is minimal alpine
- **UI + API on :8080**: the backend serves the embedded SPA (unknown paths fall back to `index.html`) alongside `/api/*`; healthcheck on `/api/health`
- **Build**: `podman build` at the repo root with named contexts (`backend`, `frontend`, `podman` fork), run via `podman run`

### 14. Rate Limiting
- **Custom rate limiting** in Go backend
- **Scope**: Per-user rate limiting for authenticated requests; auth endpoints additionally rate-limit **per IP** (unauthenticated surface)
- **Strategy**: Fixed window
- **Default**: 60 requests per minute per user; 10 per minute per IP on auth (both configurable)

### 15. File Handling
- **No file uploads** - container images pulled from registries only

### 16. Machine Registration
- Frontend uses dedicated endpoint to register machines
- **Synchronous connection test** during registration
- Connection details stored encrypted in SQLite database
- **Owner-based access**: Machine accessible by owner and shared users

### 17. First-Run Bootstrap
- **Setup flow creates the first admin** — the only bootstrap path. A fresh
  instance (users table empty) serves a setup landing page; the first admin
  is created through `POST /api/setup/user` (or the UI form riding it), and
  the creator is signed in on the spot. Afterwards the endpoint answers 409
  forever
- **No baked-in credentials**: the container image ships no database file and
  creates no default admin at build time; a fresh image has no default
  credentials of any kind
- **Database lives in `/app/data` inside the container** (`DATABASE_PATH`
  env in the Containerfile); that directory is the volume to mount
  (`-v podland-data:/app/data`) so a created admin survives container
  recreation. Host/dev default stays `./podland.db`
- **Self-lockout escape hatch**: the role self-lockout guard is worked
  around by editing the SQLite database directly, not by a tool
- **Admin-only user management rides the same creation path** (hash +
  validation + exists-check), so setup and later user creation cannot
  drift; the in-process test seeding seam (`App.SeedUserWithRole`) is a
  test-only adapter over the same path

### 18. Quadlet Volume Data Migration (2026-10-02, issue 214)
The move/copy transfer (issue 207/211/217 lineage) gains an opt-in data
phase that carries the referenced podman volumes' content from source to
target:
- **Opt-in, conditional transport**: `migrateVolumeData` on the request —
  default false, file-only transfers answer the plain JSON envelope exactly
  as before. With the flag set the response switches to the NDJSON
  streaming kit (the images-pull arrangement): per-volume progress events
  (`start`/`bytes`/`done`/`skip`), then exactly one terminal event
  (complete envelope or mapped error). Errors answered before the stream
  begins (bad param/body) stay plain. The response carries its own
  10-minute write deadline — an operation-duration fact, handler-side.
- **Phases run before anything is installed**: preflight (target emptiness
  → 409 `VOLUME_MIGRATION_TARGET_NOT_EMPTY` unless `overwriteVolumes`
  merges; size bound → 413) → a move's stop → the data itself. A data
  failure therefore leaves both machines' quadlet sets untouched. A
  migrating move stops the source's running unit BEFORE the data phase
  (exporting under a live writer would capture torn state); the
  non-migration path keeps the delete flow's stop inside the removal,
  byte-identical to its issue-211 shape.
- **Service-owned bound**: the migration bound is the same
  `VOLUME_UPLOAD_MAX_BYTES` config value the volumes domain caps populate
  uploads with, wired via `WithMaxVolumeUploadBytes`. Podman's service API
  reports NO volume content size today (verified in the fork:
  `InspectVolumeData` has no size), so the inspect-size pre-check is
  conditional — it fires only for an engine that reports one; the counting
  reader between export and import is the always-on enforcement and fails
  the stream with 413 at the first byte past the bound.
- **Emptiness via bounded export probe**: target emptiness is decided by
  exporting the target volume and reading the tar — directory entries and
  tar meta headers are framing, the first real entry proves content and
  closes the pipe mid-stream (the probe never reads a big volume to its
  end). A directory-only tar followed by a clean EOF means empty.
- **Two leases on purpose**: the data phase holds the source lease (export)
  and the target lease (create-if-absent + import) at once; export streams
  through an in-process pipe, cancellation closes the pipe so a client
  disconnect ends the export promptly.
- **Resolution ritual shared by preview and transfer**: a volume-kind
  reference resolves to the bare name itself, or through the companion
  `.volume` quadlet's `VolumeName=` (first non-empty wins), else podman's
  `systemd-<name>` default — dangling references still resolve the default
  so the preview can label them and the data phase can skip them honestly
  (skipped outcomes ride the envelope's `volumeMigration` block). Host
  bind paths never enter the migration set; the plan deduplicates by
  resolved volume name. The preview shows these facts (migratable, target
  existence, sizes when an engine reports them) flag-independently. The
  same opt-in pattern carries `Secret=` references (issue 215): the value
  is revealed on the source and recreated on the target, gated by **admin
  on the source** — a secret's value leaving a machine is a reveal, so
  the secrets API's reveal gate owns it — with same-name skips before the
  reveal and copy-only semantics (a move never removes source secrets).
  Both projections — the preview's per-reference entries and the plan's
  per-volume entries — derive from one entry point
  (`deriveVolumeResolution`); the dedup-key divergence that preceded this
  ruling (preview deduped by reference string, plan by resolved name) is
  the incident that requires it.

## Technical Stack

### Backend (Go)
- **Framework**: Standard library + custom routing
- **API**: RESTful endpoints
- **Database**: SQLite (via `github.com/mattn/go-sqlite3`)
- **Authentication**: JWT (RS256) with Authorization: Bearer header
- **Logging**: slog (structured, `log/slog`)
- **Validation**: `go-playground/validator`
- **SSH**: `golang.org/x/crypto/ssh` via forked Podman's connection.go
- **Containerization**: Podman (for integration tests and deployment)
- **Dependencies**: Go 1.21+

### Frontend (React)
- **Framework**: React 18+
- **State Management**: React Query
- **Styling**: Chakra UI with CSS variables
- **Routing**: React Router v6
- **Real-time**: EventSource API for SSE, Fetch API with AbortController for streaming
- **Error Handling**: Context-aware error display with React Query
- **Build Tool**: Vite
- **TypeScript**: Strict mode with path aliases

Feature-folder layout, DTO placement, and every kit ruling (ResourceList,
MutationDialog, stream transport, URL builders, session refresh, log
source, test kit) live in `docs/frontend-conventions.md`.

### Infrastructure
- **Delivery**: One container image (backend with embedded UI)
- **Container Runtime**: Podman (for the app image and integration tests)
- **Database**: SQLite (embedded)

## Project Layout

```
Podland/
├── AGENTS.md, CONTEXT.md, README.md
├── Containerfile, Containerfile.dev, startDevbox.sh
├── docs/                    # conventions + these decisions
├── doku/                    # API contracts (description_*_api.md)
├── backend/
│   ├── cmd/server/          # entrypoint
│   └── internal/
│       ├── app/             # router + wiring
│       ├── config/          # env-var configuration
│       ├── connections/     # cached SSH/podman connections (LRU)
│       ├── contextutils/    # context helpers (user ID extraction)
│       ├── domain/          # one package per resource: auth, containers,
│       │                    #   images, machines, quadlets, secrets, users, volumes
│       ├── encryption/      # secret encryption at rest
│       ├── errors/          # AppError + envelope error codes
│       ├── middleware/      # auth, admin, permission, rate limiting
│       ├── models/          # DB models + permission ladder
│       ├── permission/      # RequireLevel gate
│       ├── podmanerr/       # podman error classification adapter
│       ├── sshexec/         # systemctl-over-SSH exec pool
│       ├── streaming/       # NDJSON envelope + log-stream kit
│       ├── testutils/       # handler-test request builder
│       ├── web/             # endpoint envelopes (Handle, HandleUser,
│       │                    #   HandleStream) + Decode
│       └── webui/           # serves the embedded SPA
└── frontend/
    └── src/
        ├── features/        # auth, containers, images, machines,
        │                    #   quadlets, secrets, volumes
        ├── shared/          # api client, components, queries, streaming, utils
        ├── pages/           # route-level pages
        └── test/            # test kit: render harness, errors, factories, handlers
```

## Value Statements

### Simplicity
- Fail fast on errors (no retry logic)
- Authorization: Bearer header for JWT (frontend manages token storage)
- One connection per machine (not per user)
- Direct streaming with NDJSON (no buffering)

### Security
- SSH keys encrypted at rest in SQLite
- JWT tokens with RS256 asymmetric signing
- Short-lived tokens transported via Authorization: Bearer header
- Application-level permissions for machine sharing

### Performance
- Connection caching and reuse
- LRU eviction for connection management
- Per-machine dial dedup; fail fast when a connection cannot be acquired (no queuing)
- React Query for efficient data fetching and caching

### User Experience
- Real-time updates via SSE
- Progress streaming for long operations
- Offline detection with auto-reconnect
- Consistent error handling

## Security Considerations

1. **SSH Keys**: Encrypted at rest in SQLite using go-securesystemslib
2. **JWT**: Short-lived tokens, RS256 asymmetric signing, Authorization: Bearer header
3. **CORS**: Strict origin control via environment variables
4. **Input Validation**: Structured validation on all API inputs
5. **Error Handling**: No sensitive data in error responses
6. **Offline Handling**: Clear offline state, no data leakage
7. **Multi-user**: Application-level permissions, owner's SSH credentials used for all access

## Assumptions & Dependencies

### Assumptions
- Podman is pre-installed on all remote machines
- Podman Service API is accessible via Unix socket on remote machines
- Users have SSH access to remote machines via owner's credentials
- Backend runs in an environment with network access to remote machines
- Each machine has a single owner with valid SSH credentials

### Dependencies
- **Go**: 1.21+
- **React**: 18+
- **Node.js**: 18+ (for frontend development)
- **Podman**: Version matching forked Podman
- **SQLite**: 3.x

## Open Questions

1. **Monitoring**: Should Prometheus metrics be added for connection count, request rates, etc.? If so, where should they be stored?
2. **Audit Logs**: Should long-term audit logs be implemented?

---

*Originally generated from comprehensive grilling sessions; maintained as the system-level decision record.*
