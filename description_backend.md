# Podman Management Backend: Detailed Architecture

## Overview

This document provides the detailed backend architecture decisions for the Podman Management Web-App, building upon the high-level architecture described in [description.md](./description.md).

---

## Backend Structure

### Project Layout
```
backend/
├── cmd/
│   └── server/
│       └── main.go              # Entry point, server startup, dependency wiring
├── internal/
│   ├── domain/
│   │   ├── machines/           # Machine management domain
│   │   │   ├── handler.go      # HTTP handlers
│   │   │   ├── service.go      # Business logic
│   │   │   └── repository.go   # Database operations
│   │   ├── containers/          # Container management domain
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   └── repository.go
│   │   ├── images/             # Image management domain
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   └── repository.go
│   │   ├── auth/               # Authentication domain
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   └── repository.go
│   │   └── events/             # SSE event streaming domain
│   │       ├── handler.go
│   │       └── service.go
│   ├── middleware/             # HTTP middleware (auth, rate limiting, etc.)
│   │   ├── auth.go
│   │   └── ratelimit.go
│   ├── config/                 # Configuration management
│   │   └── config.go
│   ├── connections/            # SSH connection management
│   │   ├── connection.go       # Connection caching and pooling
│   │   └── podman.go           # Podman Service API wrapper
│   ├── encryption/             # Sensitive data encryption
│   │   └── encryption.go
│   └── models/                 # Database models
│       ├── machine.go
│       ├── user.go
│       └── access.go
├── pkg/
│   └── utils/                  # Shared utilities
├── go.mod
├── go.sum
└── Dockerfile
```

### Domain-Driven Design
- Each domain (`machines`, `containers`, `images`, `auth`, `events`) is self-contained
- **Dependency injection at handler layer**: HTTP handlers receive services as dependencies
- **Cross-domain coordination**: Handlers coordinate between domain services (e.g., machine lookup + container operation)

---

## Connection Management

### SSH Connection Model
- **One connection per machine**: Each machine has at most one active SSH connection
- **Connection cache**: In-memory cache keyed by `machineID`
- **Cache behavior**:
  - Connections are reused for multiple requests to the same machine
  - Idle connections are closed after configurable timeout (`SSH_IDLE_TIMEOUT_SECONDS`)
  - Streaming operations (e.g., image pull) mark connections as "active" to prevent idle closure
- **Global limit**: Maximum number of concurrent machine connections (`MAX_CONCURRENT_MACHINES=100`, configurable)

### Connection Lifecycle
1. **Request arrives** for a machine
2. **Check cache** for existing connection to that machine
3. **If exists** and not at global limit:
   - Mark connection as "active"
   - Reuse connection
4. **If cache full** (global limit reached):
   - Find **least recently used idle connection** (not currently serving a request)
   - **Close it** to free a slot
   - Create new connection for requested machine
5. **If all connections active** (no idle connections to evict):
   - **Queue the request** until a connection becomes free
   - Queue timeout: configurable via `CONNECTION_QUEUE_TIMEOUT_SECONDS`

### Connection Implementation
- Uses forked Podman's `~/repos/podman/pkg/bindings/connection.go` for SSH tunneling
- Backend establishes connection to remote Podman Service API via SSH tunnel
- Podman Service API accessed via HTTP over Unix socket (forwarded through SSH)

---

## Multi-User Access Model

### Ownership and Sharing
- Each machine is **registered by an owner** (user)
- Owner's **SSH private key** is stored encrypted in SQLite
- Machine can be **shared with other users** via `machine_access` table
- **All users access the machine through the owner's SSH credentials**
- Application-level permissions (read/write/admin) enforced by backend

### Database Schema

```sql
-- Users
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Machines
CREATE TABLE machines (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    description TEXT,
    hostname TEXT NOT NULL,
    port INTEGER DEFAULT 22,
    ssh_username TEXT NOT NULL,
    ssh_key_encrypted BLOB NOT NULL,
    ssh_key_fingerprint TEXT,
    podman_socket_path TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (owner_id) REFERENCES users(id)
);

-- Machine access permissions
CREATE TABLE machine_access (
    machine_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    permission TEXT NOT NULL CHECK(permission IN ('read', 'write', 'admin')),
    granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (machine_id, user_id),
    FOREIGN KEY (machine_id) REFERENCES machines(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);
```

---

## Authentication

### JWT Implementation
- **Algorithm**: RS256 (asymmetric signing)
- **Storage**: HTTP-only cookies
- **Key storage**: File-based (PEM files)
  - Private key path: `JWT_PRIVATE_KEY_PATH` (default: `./jwt-private.pem`)
  - Public key path: `JWT_PUBLIC_KEY_PATH` (default: `./jwt-public.pem`)
  - **Auto-generation**: If key files don't exist on startup, generate new key pair

### JWT Middleware
- Applied to all protected routes
- Validates JWT from HTTP-only cookie
- Extracts user claims and attaches to request context
- On expiration: Returns 401, frontend redirects to login

### Endpoints
```
POST /api/auth/login       # Authenticate, return JWT in HTTP-only cookie
POST /api/auth/logout      # Clear cookie
GET  /api/auth/status      # Check authentication status
```

---

## API Endpoints

### Machine Management
```
POST   /api/machines              # Register new machine (with synchronous connection test)
GET    /api/machines              # List all machines user has access to
GET    /api/machines/{id}         # Get machine details
PUT    /api/machines/{id}         # Update machine connection details
DELETE /api/machines/{id}         # Remove machine
GET    /api/machines/{id}/access  # List users with access to machine
POST   /api/machines/{id}/access  # Share machine with another user
DELETE /api/machines/{id}/access/{userId}  # Revoke access
```

### Container Operations
```
GET    /api/machines/{id}/containers                    # List containers
POST   /api/machines/{id}/containers                     # Create container
GET    /api/machines/{id}/containers/{containerId}     # Get container details
POST   /api/machines/{id}/containers/{containerId}/start  # Start container
POST   /api/machines/{id}/containers/{containerId}/stop   # Stop container
DELETE /api/machines/{id}/containers/{containerId}     # Remove container
```

### Image Operations
```
GET    /api/machines/{id}/images           # List images
POST   /api/machines/{id}/images/pull     # Pull image (streaming with progress)
DELETE /api/machines/{id}/images/{imageId} # Remove image
```

### Events
```
GET /api/machines/{id}/events  # SSE endpoint for Podman events (container start/stop, etc.)
```

---

## Progress Streaming

### Image Pull and Other Long-Running Operations
- **Transport**: HTTP Streaming with `Content-Type: application/json`
- **Format**: Newline-delimited JSON (NDJSON-style)
- **No buffering**: Progress updates streamed immediately as received from Podman Service API
- **Progress structure**:
  ```json
  {
    "operationId": "abc123",
    "type": "imagePull",
    "machineId": "machine-42",
    "state": {
      "current": 42,
      "total": 100,
      "status": "downloading",
      "message": "Downloading layer 3/5"
    },
    "timestamp": "2026-06-30T18:41:00Z"
  }
  ```

### Error Handling in Stream
- Errors sent as **final JSON object** in the stream:
  ```json
  {
    "operationId": "abc123",
    "type": "error",
    "error": {
      "code": "CONNECTION_LOST",
      "message": "SSH connection dropped"
    }
  }
  ```
- Connection closed after error

---

## Error Handling

### Response Format
All error responses follow this structure:
```json
{
  "requestId": "req-abc123",
  "error": {
    "message": "Human-readable error message",
    "code": "ERROR_CODE"
  }
}
```

### Error Categories
- **4xx (Client errors)**: Detailed messages with specific codes (e.g., `MACHINE_NOT_FOUND`, `INVALID_CREDENTIALS`)
- **5xx (Server errors)**: Generic messages with code `INTERNAL_ERROR` or `SERVICE_UNAVAILABLE`

### Error Codes
| Category | Code | Description |
|----------|------|-------------|
| Machines | `MACHINE_NOT_FOUND` | Machine ID doesn't exist or user lacks access |
| Machines | `MACHINE_ALREADY_EXISTS` | Machine name conflict for user |
| Machines | `CONNECTION_FAILED` | Could not establish SSH connection |
| Machines | `CONNECTION_TEST_TIMEOUT` | Connection test timed out |
| Auth | `INVALID_CREDENTIALS` | Wrong username/password |
| Auth | `TOKEN_EXPIRED` | JWT token expired |
| Auth | `TOKEN_INVALID` | Malformed or tampered JWT |
| Rate Limiting | `RATE_LIMIT_EXCEEDED` | User exceeded rate limit |
| Containers | `CONTAINER_NOT_FOUND` | Container doesn't exist |
| Containers | `CONTAINER_ALREADY_RUNNING` | Cannot start running container |
| Images | `IMAGE_NOT_FOUND` | Image doesn't exist |
| Images | `IMAGE_PULL_FAILED` | Failed to pull image |
| Generic | `INTERNAL_ERROR` | Unexpected server error (5xx) |
| Generic | `SERVICE_UNAVAILABLE` | Service temporarily unavailable (5xx) |

---

## Rate Limiting

### Implementation
- **Scope**: Per-user rate limiting for authenticated requests
- **Strategy**: Fixed window (count requests in each time window)
- **Limit**: `RATE_LIMIT_REQUESTS_PER_USER_PER_MINUTE=60` (configurable)
- **Middleware**: Applied after authentication middleware

### Behavior
- When limit exceeded: Return 429 Too Many Requests with error code `RATE_LIMIT_EXCEEDED`
- Rate limit headers in response:
  - `X-RateLimit-Limit`: Maximum requests per window
  - `X-RateLimit-Remaining`: Requests remaining in current window
  - `X-RateLimit-Reset`: Seconds until window resets

---

## Machine Registration

### Flow
1. User submits machine details: `name`, `hostname`, `port`, `sshUsername`, `sshPrivateKey`
2. Backend validates required fields
3. Backend checks `name` is unique for the user
4. Backend validates SSH private key is valid PEM format
5. **Synchronous connection test**:
   - Backend attempts to establish SSH connection using provided credentials
   - Tests connection to Podman Service API via `connection.go`
   - Timeout: `MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS=10` (configurable)
   - If connection fails, return 400 with error code `CONNECTION_FAILED`
6. If connection succeeds, **encrypt SSH private key** and save machine to database
7. Return 201 Created with machine details

### Encryption
- Uses `github.com/secure-systems-lab/go-securesystemslib`
- SSH private keys encrypted before storage in SQLite
- Same encryption for any other sensitive data

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `DATABASE_PATH` | `./podland.db` | SQLite database file path |
| `JWT_PRIVATE_KEY_PATH` | `./jwt-private.pem` | Path to JWT private key |
| `JWT_PUBLIC_KEY_PATH` | `./jwt-public.pem` | Path to JWT public key |
| `JWT_EXPIRATION_MINUTES` | `60` | JWT token expiration |
| `SSH_IDLE_TIMEOUT_SECONDS` | `900` | SSH connection idle timeout (15 min) |
| `MAX_CONCURRENT_MACHINES` | `100` | Maximum concurrent machine connections |
| `CONNECTION_QUEUE_TIMEOUT_SECONDS` | `30` | Max wait time for queued connection |
| `MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS` | `10` | Timeout for machine connection test |
| `RATE_LIMIT_REQUESTS_PER_USER_PER_MINUTE` | `60` | Rate limit requests per minute |
| `ALLOWED_ORIGINS` | (none) | CORS allowed origins (comma-separated) |

---

## Logging

### Format
- Uses Logrus library
- JSON format for production
- Human-readable for development
- Includes `requestId` in all log entries for tracing

### Log Levels
- `DEBUG`: Detailed request/response logging, connection lifecycle
- `INFO`: Startup, shutdown, major events
- `WARN`: Configuration issues, retryable errors
- `ERROR`: Permanent failures, unexpected errors

---

## Testing Strategy

### Unit Tests
- Mocked Podman Service API responses
- Test handlers, services, and repositories in isolation
- Use in-memory SQLite for repository tests

### Integration Tests
- Real Podman instance in container
- Orchestrated using Podman pods
- Test full request flow from HTTP to Podman Service API

### Connection Management Tests
- Test connection caching and reuse
- Test LRU eviction behavior
- Test queue behavior when limit reached
- Test idle timeout closure

---

## Open Questions

1. **Frontend Integration**: How should the frontend handle streaming responses (image pull progress)?
2. **SSE Implementation**: Should the `/events` endpoint use the same connection cache or separate connections?
3. **Health Checks**: Should we add a `/health` endpoint for monitoring?
4. **Metrics**: Should we add Prometheus metrics for connection count, request rates, etc.?

---

## Next Steps

1. Implement Go backend skeleton with configuration
2. Set up SQLite database migrations
3. Implement JWT authentication middleware
4. Implement connection management (cache, pooling, LRU eviction)
5. Implement machine domain (registration, listing, etc.)
6. Implement container and image domains
7. Implement SSE endpoint for events
8. Add rate limiting middleware
9. Write unit and integration tests
10. Containerize backend with Docker/Podman

---

*Generated from grilling session*
*Last updated: 2026-06-30*
