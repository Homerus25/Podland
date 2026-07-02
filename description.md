# Podman Management Web-App: Architecture Summary

## Overview
A web application for managing Podman on remote machines, consisting of:
- **Backend**: Go service using Podman's Service API
- **Frontend**: React application
- **Database**: SQLite for storing machine metadata

---

## Core Architecture Decisions

### 1. Podman Integration
- **Forked Podman** extended for custom needs (Project folder: ~/repos/podman/)
- **Backend interacts via Podman Service API** (REST/gRPC over Unix socket)
- **No CLI integration** - only Go API usage

### 2. Remote Machine Access
- To establish connections to remote machines see ~/repos/podman/pkg/bindings/connection.go
- **Podman must be pre-installed** on remote machines
- **No deployment of Podman** - backend assumes it exists

### 3. Concurrency & Performance
- **Goroutines with semaphore** for limiting concurrent SSH tunnels
- **React Query** for frontend state management
- **Fail fast** on connection failures/timeouts (no retry logic)

### 4. Authentication & Security
- **JWT tokens** (RS256 asymmetric keys)
  - Stored in HTTP-only cookies
  - Frontend redirects to login page on expiration
- **CORS**: Environment variables for allowed origins
- **Sensitive data encryption**: `github.com/secure-systems-lab/go-securesystemslib`
  - Encrypts SSH private keys and other secrets before SQLite storage

### 5. Real-Time Updates
- **Server-Sent Events (SSE)** for real-time Podman events
  - Container start/stop, image pulls, etc.
- **Abortable operations** with progress tracking

### 6. Error Handling
- **Frontend**:
  - Detailed error messages for client errors (4xx)
  - Generic error messages for server errors (5xx)
- **Backend**:
  - Logs via Logrus
  - Fail fast on SSH/Podman failures

### 7. Configuration
- **Go backend**: Environment variables only
- **No config files** for sensitive data

### 8. Theming
- **CSS variables** with theme provider (e.g., Material-UI, Chakra)
- Supports dark mode

### 9. Offline Handling
- Redirect to offline page when connection to backend is lost

### 10. Input Validation
- **Go backend**: Struct tags + `github.com/go-playground/validator`
- **Frontend**: React Query for server state management

### 11. Testing Strategy
- **Unit tests**: Mocked Podman responses
- **Integration tests**: Real Podman instance in container
  - Orchestrated using **Podman pods**
  - Can use `docker-compose` or custom scripts for setup

### 12. Deployment & Distribution
- **Backend**: Distributed as container image
  - Run via `podman run` on remote machines
- **Frontend**: Served through Caddy reverse proxy
  - Caddy handles both frontend and backend routing

### 13. Rate Limiting
- Custom rate limiting logic in Go backend

### 14. File Handling
- **No file uploads** - container images pulled from registries only

### 15. Machine Registration
- Frontend uses dedicated endpoint to register machines
- Connection details stored encrypted in SQLite database

---

## Technical Stack

### Backend (Go)
- **Framework**: Standard library + custom routing
- **API**: RESTful endpoints
- **Database**: SQLite (via `github.com/mattn/go-sqlite3`)
- **Authentication**: JWT (RS256)
- **Logging**: Logrus
- **Validation**: `go-playground/validator`
- **SSH**: `golang.org/x/crypto/ssh`
- **Containerization**: Podman (for integration tests)

### Frontend (React)
- **Framework**: React 18+
- **State Management**: React Query
- **Styling**: CSS variables + theme provider (Material-UI/Chakra)
- **Real-time**: EventSource API for SSE
- **Error Handling**: Context-aware error display
- **Build Tool**: Vite or Create React App

### Infrastructure
- **Reverse Proxy**: Caddy
- **Container Runtime**: Podman (for both backend and integration tests)
- **Database**: SQLite (embedded)

---

## Key Endpoints

### Machine Management
- `POST /api/machines` - Register a new remote machine
- `GET /api/machines` - List all registered machines
- `GET /api/machines/{id}` - Get machine details
- `PUT /api/machines/{id}` - Update machine connection details
- `DELETE /api/machines/{id}` - Remove a machine

### Podman Operations
- `GET /api/machines/{id}/containers` - List containers
- `POST /api/machines/{id}/containers/start` - Start container
- `POST /api/machines/{id}/containers/stop` - Stop container
- `POST /api/machines/{id}/containers/create` - Create container
- `DELETE /api/machines/{id}/containers/{containerId}` - Remove container
- `GET /api/machines/{id}/images` - List images
- `POST /api/machines/{id}/images/pull` - Pull image
- `GET /api/machines/{id}/events` - SSE endpoint for Podman events

### Authentication
- `POST /api/auth/login` - Login (returns JWT in HTTP-only cookie)
- `POST /api/auth/logout` - Logout (clears cookie)
- `GET /api/auth/status` - Check authentication status

---

## Security Considerations

1. **SSH Keys**: Encrypted at rest in SQLite
2. **JWT**: Short-lived tokens, RS256 asymmetric signing
3. **CORS**: Strict origin control via environment variables
4. **Input Validation**: Structured validation on all API inputs
5. **Error Handling**: No sensitive data in error responses
6. **Offline Handling**: Clear offline state, no data leakage

---

## Development Workflow

1. **Backend Development**:
   - Implement API endpoints
   - Add unit tests with mocked Podman
   - Set up integration test environment

2. **Frontend Development**:
   - Build UI components
   - Integrate with backend API
   - Implement real-time updates via SSE

3. **Testing**:
   - Unit tests: Mocked Podman
   - Integration tests: Real Podman in container
   - E2E tests: Frontend + backend + Podman in Podman pod

4. **Deployment**:
   - Build backend container image
   - Deploy to remote machines via `podman run`
   - Serve frontend through Caddy

---

## Assumptions & Dependencies

### Assumptions
- Podman is pre-installed on all remote machines
- Users have SSH access to remote machines
- Backend runs in an environment with network access to remote machines

### Dependencies
- **Go**: 1.21+
- **React**: 18+
- **Podman**: Version matching forked Podman
- **Caddy**: For reverse proxy
- **SQLite**: 3.x
- **Libraries**: See respective `go.mod` and `package.json`

---

## Open Questions & Future Considerations

1. **Rate Limiting Strategy**: Custom implementation details TBD
2. **Monitoring**: Need to add Prometheus metrics endpoint
3. **Scaling**: Horizontal scaling of backend service
4. **Backup**: SQLite backup strategy for production
5. **Multi-user**: Role-based access control (RBAC) for teams
6. **Audit Logs**: Long-term storage of audit logs
7. **Notifications**: Email/Slack alerts for critical events

---

## Next Steps

1. Implement Go backend skeleton with authentication
2. Set up SQLite database schema
3. Implement machine registration endpoint
4. Build React frontend scaffolding
5. Set up integration test environment with Podman pod
6. Implement SSE endpoint for real-time updates
7. Add encryption layer for sensitive data
8. Configure Caddy for development environment

---

*Last updated: 2026-06-30*
*Generated from grilling session*
