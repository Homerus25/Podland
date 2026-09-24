# Podland

A web application for managing [Podman](https://podman.io/) containers on remote machines.

## Overview

Podland is a full-stack web app that lets you register remote machines, manage their
Podman containers, images, and quadlets, and share access to those machines with
other users — all from a single browser interface.

**Stack**

- **Backend**: Go service talking to Podman's Service API over SSH tunnels
- **Frontend**: React 18 + Chakra UI + Vite
- **Database**: SQLite for machine metadata, users, and permissions
- **Auth**: JWT (RS256) with revocable refresh sessions

## Features

- Register remote machines and connect via the owner's SSH credentials
- Machines are managed through Podman's Unix socket, exposed over an SSH tunnel
- Manage containers: create, start, stop, delete, stream logs
- Browse and filter images
- Manage quadlets with lifecycle integration over SSH/systemctl
- Share machines with other users (`read` / `write` / `admin` permissions)
- Multi-user authentication with refresh sessions and rate-limited login

## Requirements

- Go 1.22+ (backend)
- Node.js 18+ (frontend)
- Podman pre-installed on every managed machine

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

## Podman Integration

Podland tightly integrates with Podman: it talks to the Podman Service API
(REST over the Unix socket on each remote machine, accessed via SSH tunnel)
using a patched fork of Podman's Go bindings —
[github.com/Homerus25/podman](https://github.com/Homerus25/podman).
Podman must be pre-installed on every managed machine.

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
