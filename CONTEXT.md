# Podman Management Web-App: Complete Architecture

## Overview

A full-stack web application for managing Podman on remote machines. This document consolidates all architectural decisions made during the grilling sessions.

**System Components:**
- **Backend**: Go service using Podman's Service API
- **Frontend**: React 18+ application with Chakra UI
- **Database**: SQLite for storing machine metadata and user data
- **Reverse Proxy**: Caddy for serving frontend and routing to backend

---

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
- **Goroutines with semaphore** for limiting concurrent SSH tunnels
- **Connection per machine**: Each machine has at most one active SSH connection
- **Global limit**: Maximum concurrent machine connections = 100 (configurable)
- **LRU eviction**: When limit reached, close least recently used idle connection
- **Request queuing**: If all connections active, queue new requests
- **React Query** for frontend state management and caching
- **Fail fast** on connection failures/timeouts (no retry logic)

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
- **Frontend**:
  - Detailed error messages for client errors (4xx)
  - Generic error messages for server errors (5xx)
  - Context-based error display
  - Global 401 interceptor for token expiration
- **Backend**:
  - Logs via Logrus
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
- **Backend**: Distributed as container image, run via `podman run`
- **Frontend**: Built with Vite, served through Caddy reverse proxy
- **Caddy**: Handles both frontend and backend routing

### 14. Rate Limiting
- **Custom rate limiting** in Go backend
- **Scope**: Per-user rate limiting for authenticated requests
- **Strategy**: Fixed window
- **Default**: 60 requests per minute per user (configurable)

### 15. File Handling
- **No file uploads** - container images pulled from registries only

### 16. Machine Registration
- Frontend uses dedicated endpoint to register machines
- **Synchronous connection test** during registration
- Connection details stored encrypted in SQLite database
- **Owner-based access**: Machine accessible by owner and shared users

---

## Technical Stack

### Backend (Go)
- **Framework**: Standard library + custom routing
- **API**: RESTful endpoints
- **Database**: SQLite (via `github.com/mattn/go-sqlite3`)
- **Authentication**: JWT (RS256) with Authorization: Bearer header
- **Logging**: Logrus
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

### Infrastructure
- **Reverse Proxy**: Caddy
- **Container Runtime**: Podman (for both backend and integration tests)
- **Database**: SQLite (embedded)

---

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
- Request queuing when limits reached
- React Query for efficient data fetching and caching

### User Experience
- Real-time updates via SSE
- Progress streaming for long operations
- Offline detection with auto-reconnect
- Consistent error handling

---

## Project Structure

```
Podland/
├── description.md                    # High-level architecture
├── description_backend.md           # Backend details
├── doku/
│   ├── description.md               # This file - complete architecture
│   ├── description_backend.md        # Backend implementation guide
│   ├── description_frontend.md       # Frontend implementation guide
│   ├── description_api.md            # API contract (shared conventions)
│   ├── description_auth_api.md       # Auth endpoints
│   ├── description_machines_api.md   # Machine + event endpoints
│   ├── description_containers_api.md # Container endpoints
│   ├── description_images_api.md     # Image endpoints
│   └── description_quadlets_api.md   # Quadlet endpoints
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   ├── internal/
│   │   ├── domain/
│   │   │   ├── machines/
│   │   │   ├── containers/
│   │   │   ├── images/
│   │   │   ├── auth/
│   │   │   └── events/
│   │   ├── middleware/
│   │   ├── connections/
│   │   ├── config/
│   │   ├── encryption/
│   │   └── models/
│   ├── pkg/
│   └── go.mod
└── frontend/
    ├── public/
    └── src/
        ├── features/
        │   ├── auth/
        │   ├── machines/
        │   ├── containers/
        │   ├── images/
        │   └── events/
        ├── shared/
        ├── pages/
        ├── App.tsx
        └── main.tsx
```

---

## Security Considerations

1. **SSH Keys**: Encrypted at rest in SQLite using go-securesystemslib
2. **JWT**: Short-lived tokens, RS256 asymmetric signing, Authorization: Bearer header
3. **CORS**: Strict origin control via environment variables
4. **Input Validation**: Structured validation on all API inputs
5. **Error Handling**: No sensitive data in error responses
6. **Offline Handling**: Clear offline state, no data leakage
7. **Multi-user**: Application-level permissions, owner's SSH credentials used for all access

---

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
- **Caddy**: For reverse proxy
- **SQLite**: 3.x

---

## Open Questions

1. **Monitoring**: Should Prometheus metrics be added for connection count, request rates, etc.?
2. **Health Checks**: Should a `/health` endpoint be added for monitoring?
3. **Metrics Storage**: If metrics are added, where should they be stored?
4. **Audit Logs**: Should long-term audit logs be implemented?

---

## Glossary

| Term | Definition |
|------|------------|
| **Machine** | A remote server with Podman installed that can be managed through this application |
| **Owner** | The user who registered a machine and provided its SSH credentials |
| **Shared User** | A user who has been granted access to a machine by its owner |
| **Connection Cache** | In-memory cache of SSH connections to remote machines |
| **Podman Service API** | The REST API provided by Podman for managing containers and images |
| **SSE** | Server-Sent Events, a standard for push notifications from server to client |

---

*Generated from comprehensive grilling sessions*
*Last updated: 2026-06-30*
