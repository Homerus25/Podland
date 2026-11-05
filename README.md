# Podland

A web application for managing [Podman](https://podman.io/) containers on remote hosts.

## Overview

Podland is a full-stack web app that lets you register remote hosts, manage their
Podman containers, images, and quadlets, and share access to those hosts with
other users — all from a single browser interface.

**Stack**

- **Backend**: Go service talking to Podman's Service API over SSH tunnels
- **Frontend**: React 18 + Chakra UI + Vite
- **Database**: SQLite for host metadata, users, and permissions
- **Auth**: JWT (RS256) with revocable refresh sessions

## Features

- Register remote hosts and connect via the owner's SSH credentials
- Hosts are managed through Podman's Unix socket, exposed over an SSH tunnel
- Manage containers: create, start, stop, delete, stream logs
- Browse and filter images
- Manage quadlets with lifecycle integration over SSH/systemctl
- Share hosts with other users (`read` / `write` / `admin` permissions)
- Multi-user authentication with refresh sessions and rate-limited login

## Requirements

- Go 1.22+ (backend)
- Node.js 18+ (frontend)
- Podman pre-installed on every managed host

## Getting Started

```bash
# Backend
cd backend
make run

# Frontend
cd frontend
npm install
npm run dev
```

In development the frontend runs on Vite's dev server (:3000) proxying
`/api` to the backend on :8080.

## One-Container Delivery

For deployment Podland ships as a **single container image**: the Go
backend binary with the compiled React SPA embedded. One build, one image,
one `podman run` — the UI and the API are both served on **:8080**
(healthcheck on `/api/health`).

```bash
podman build \
  --build-context backend=backend \
  --build-context frontend=frontend \
  --build-context podman=$HOME/repos/podman \
  -t podland .

podman run -p 8080:8080 -v podland-data:/app/data podland
```

`--build-context podman` points at a local checkout of the patched Podman
bindings fork ([github.com/Homerus25/podman](https://github.com/Homerus25/podman))
that `backend/go.mod` carries as a `replace` directive. See the root
[Containerfile](Containerfile) for the build stages.

A fresh image ships **no database and no default credentials**: on first
boot the browser opens the setup page where you create the first admin.
The SQLite database lives in `/app/data` — mount that directory as a
volume (as above) so the created admin and all data survive container
recreation.

## Configuration

The backend reads its configuration from environment variables at startup
(`backend/internal/config/config.go`). The `VITE_*` variables are build-time:
Vite bakes them into the frontend bundle (see `frontend/.env.example`).

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP port the server listens on |
| `DATABASE_PATH` | `./podland.db` | SQLite database file (`/app/data/podland.db` in the container image) |
| `JWT_PRIVATE_KEY_PATH` | `./jwt-private.pem` | Path to the RS256 private key used to sign access tokens |
| `JWT_PUBLIC_KEY_PATH` | `./jwt-public.pem` | Path to the public key used to verify access tokens |
| `JWT_EXPIRATION_MINUTES` | `60` | Access-token lifetime in minutes |
| `JWT_ISSUER` | `podland` | `iss` claim written into issued tokens |
| `SESSION_TTL_DAYS` | `30` | Days a login's refresh session stays redeemable |
| `SESSION_REMEMBER_TTL_DAYS` | `90` | Days a "remember this device" login's refresh session stays redeemable (must be >= `SESSION_TTL_DAYS`) |
| `SSH_IDLE_TIMEOUT_SECONDS` | `900` | Idle timeout for SSH tunnels to managed hosts |
| `MAX_CONCURRENT_HOSTS` | `100` | Maximum number of concurrent host connections |
| `HOST_CONNECTION_TEST_TIMEOUT_SECONDS` | `10` | Timeout for the connection test when registering a host |
| `HOST_BLOCK_PRIVATE_RANGES` | `false` | Opt-in egress block on the host registration/update probe target: reject hostnames whose IP (literal or resolved) falls in loopback, RFC1918, link-local, CGNAT, IPv6 ULA, or unspecified ranges with 400 `HOST_NOT_ALLOWED` (unresolvable fails closed). Off by default — LAN/localhost podman hosts are the primary use case |
| `RATE_LIMIT_REQUESTS_PER_USER_PER_MINUTE` | `60` | Authenticated API requests allowed per minute per user |
| `RATE_LIMIT_AUTH_REQUESTS_PER_IP_PER_MINUTE` | `10` | Login/refresh requests allowed per minute per client IP |
| `RATE_LIMIT_AUTH_FAILURE_BUDGET` | `5` | Failed password attempts allowed per window before throttling (HTTP 429) |
| `RATE_LIMIT_AUTH_FAILURE_WINDOW_MINUTES` | `15` | Credential-failure window in minutes |
| `ALLOWED_ORIGINS` | (empty) | Comma-separated list of CORS allowed origins. Empty = same-origin only (no CORS headers); `*` = wildcard without credentials; specific origins = exact match with credentials |
| `HSTS_MAX_AGE_SECONDS` | `0` | Strict-Transport-Security max-age; `0` omits the header (set only behind TLS termination) |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, or `error` |
| `LOG_FORMAT` | `json` | Log output format: `json` or `text` |
| `ENCRYPTION_KEY` | — (required) | Base64-encoded 32-byte key (AES-256-GCM) encrypting stored SSH private keys |
| `VOLUME_UPLOAD_MAX_BYTES` | `2147483648` (2 GiB) | Maximum size of a volume populate upload |
| `VITE_API_BASE_URL` | `/api` | Base URL the frontend uses for API calls (build-time) |
| `VITE_APP_NAME` | `Podland` | Application name shown in the UI (build-time) |
| `VITE_DEFAULT_THEME` | `system` | Initial color mode: `light`, `dark`, or `system` (build-time) |

## Podman Integration

Podland tightly integrates with Podman: it talks to the Podman Service API
(REST over the Unix socket on each remote host, accessed via SSH tunnel)
using a patched fork of Podman's Go bindings —
[github.com/Homerus25/podman](https://github.com/Homerus25/podman).
Podman must be pre-installed on every managed host.

## License

This software is licensed under a modified PolyForm Small Business License —
see [LICENSE](LICENSE). In short:

- Free to use, including commercially, as long as you manage **no more than
  two machines** with it.
- Managing more than two machines requires a commercial license.

## Disclaimer

**This software is under active development and is NOT production ready.**
Features may change or break without notice, and there are known and unknown
bugs. Use at your own risk. Do not use it in production environments or for
workloads that matter.
