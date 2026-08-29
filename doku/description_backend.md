# Podman Management Backend: Implementation Guide

## Overview

This document provides the complete backend implementation specification for the Podman Management Web-App. It contains all the decisions, configurations, and technical details needed to implement the Go backend service.

---

## Project Structure

```
backend/
├── cmd/
│   └── server/
│       └── main.go              # Entry point, server startup, dependency wiring
├── internal/
│   ├── domain/
│   │   ├── machines/           # Machine management domain
│   │   │   ├── handler.go      # HTTP handlers for machine endpoints
│   │   │   ├── service.go      # Business logic for machines
│   │   │   ├── repository.go   # Database operations for machines
│   │   │   └── types.go        # Machine-specific types
│   │   ├── containers/          # Container management domain
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   ├── repository.go
│   │   │   └── types.go
│   │   ├── images/             # Image management domain
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   ├── repository.go
│   │   │   └── types.go
│   │   ├── quadlets/            # Quadlet management domain
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   ├── repository.go
│   │   │   └── types.go
│   │   ├── secrets/             # Secret management domain (proxied to podman; no local repository — secrets are never persisted by the backend)
│   │   │   ├── handler.go      # HTTP handlers for secret endpoints
│   │   │   ├── service.go      # Business logic: podman bindings calls + permission gate for reveal
│   │   │   └── dto.go          # Request/response DTOs (metadata-only by default; value only on admin reveal)
│   │   ├── auth/               # Authentication domain
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   └── repository.go
│   │   └── events/             # SSE event streaming domain
│   │       ├── handler.go
│   │       └── service.go
│   ├── middleware/             # HTTP middleware
│   │   ├── auth.go             # JWT authentication middleware
│   │   ├── ratelimit.go        # Rate limiting middleware
│   │   ├── logging.go          # Request logging middleware
│   │   └── requestid.go        # Request ID middleware
│   ├── connections/            # SSH connection management
│   │   ├── cache.go            # Connection caching with LRU eviction
│   │   ├── manager.go          # Connection lifecycle management
│   │   └── podman.go           # Podman Service API wrapper
│   ├── config/                 # Configuration management
│   │   └── config.go           # Environment variable parsing
│   ├── encryption/             # Sensitive data encryption
│   │   └── encryption.go       # Encryption/decryption utilities
│   ├── errors/                 # Error handling
│   │   ├── errors.go           # Custom error types
│   │   └── handlers.go         # HTTP error response helpers
│   └── models/                 # Database models
│       ├── machine.go
│       ├── user.go
│       └── access.go
├── pkg/
│   └── utils/                  # Shared utilities
│       └── validation.go      # Validation helpers
├── migrations/                 # Database migrations
│   ├── 001_initial_schema.sql
│   └── 002_...sql
├── scripts/                    # Utility scripts
│   └── generate-keys.sh        # JWT key generation
├── go.mod
├── go.sum
├── Dockerfile
└── .env.example
```

---

## Configuration

### Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `PORT` | `8080` | No | HTTP server port |
| `DATABASE_PATH` | `./podland.db` | No | SQLite database file path |
| `JWT_PRIVATE_KEY_PATH` | `./jwt-private.pem` | No | Path to JWT private key (PEM format) |
| `JWT_PUBLIC_KEY_PATH` | `./jwt-public.pem` | No | Path to JWT public key (PEM format) |
| `JWT_EXPIRATION_MINUTES` | `60` | No | JWT token expiration in minutes |
| `JWT_ISSUER` | `podland` | No | JWT issuer claim |
| `SSH_IDLE_TIMEOUT_SECONDS` | `900` | No | SSH connection idle timeout (15 minutes) |
| `MAX_CONCURRENT_MACHINES` | `100` | No | Maximum concurrent machine connections |
| `CONNECTION_QUEUE_TIMEOUT_SECONDS` | `30` | No | Max wait time for queued connection requests |
| `MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS` | `10` | No | Timeout for machine connection test during registration |
| `RATE_LIMIT_REQUESTS_PER_USER_PER_MINUTE` | `60` | No | Rate limit: requests per minute per user |
| `ALLOWED_ORIGINS` | (empty) | No | CORS allowed origins (comma-separated) |
| `LOG_LEVEL` | `info` | No | Logging level: debug, info, warn, error |
| `LOG_FORMAT` | `json` | No | Logging format: json, text |

---

## Database Schema

### SQLite Tables

```sql
-- Users table
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Machines table
CREATE TABLE IF NOT EXISTS machines (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    description TEXT,
    hostname TEXT NOT NULL,
    port INTEGER DEFAULT 22,
    ssh_username TEXT NOT NULL,
    ssh_key_encrypted BLOB NOT NULL,
    ssh_key_fingerprint TEXT,
    podman_socket_path TEXT DEFAULT '/run/podman/podman.sock',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Machine access permissions
CREATE TABLE IF NOT EXISTS machine_access (
    machine_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    permission TEXT NOT NULL CHECK(permission IN ('read', 'write', 'admin')),
    granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (machine_id, user_id),
    FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_machines_owner_id ON machines(owner_id);
CREATE INDEX IF NOT EXISTS idx_machines_name ON machines(name);
CREATE INDEX IF NOT EXISTS idx_machine_access_machine_id ON machine_access(machine_id);
CREATE INDEX IF NOT EXISTS idx_machine_access_user_id ON machine_access(user_id);
```

### Database Models (Go)

```go
// models/user.go
type User struct {
    ID           string    `json:"id" gorm:"primaryKey"`
    Username     string    `json:"username" gorm:"unique;not null"`
    PasswordHash string    `json:"-" gorm:"not null"`
    CreatedAt    time.Time `json:"createdAt"`
}

// models/machine.go
type Machine struct {
    ID                string         `json:"id" gorm:"primaryKey"`
    Name              string         `json:"name" gorm:"not null"`
    OwnerID           string         `json:"ownerId" gorm:"not null;index"`
    Owner             *User          `json:"owner,omitempty" gorm:"foreignKey:OwnerID"`
    Description       string         `json:"description"`
    Hostname          string         `json:"hostname" gorm:"not null"`
    Port              int            `json:"port" gorm:"default:22"`
    SSHUsername       string         `json:"sshUsername" gorm:"not null"`
    SSHKeyEncrypted   []byte         `json:"-" gorm:"not null"`
    SSHKeyFingerprint string         `json:"sshKeyFingerprint"`
    PodmanSocketPath  string         `json:"podmanSocketPath" gorm:"default:'/run/podman/podman.sock'"`
    CreatedAt         time.Time      `json:"createdAt"`
    UpdatedAt         time.Time      `json:"updatedAt"`
}

// models/access.go
type MachineAccess struct {
    MachineID  string    `json:"machineId" gorm:"primaryKey"`
    UserID     string    `json:"userId" gorm:"primaryKey"`
    User       *User     `json:"user,omitempty" gorm:"foreignKey:UserID"`
    Permission string    `json:"permission" gorm:"not null;check:permission IN ('read', 'write', 'admin')"`
    GrantedAt  time.Time `json:"grantedAt"`
}

// Permission constants
const (
    PermissionRead  = "read"
    PermissionWrite = "write"
    PermissionAdmin = "admin"
)
```

---

## Connection Management

### SSH Connection Cache

```go
// connections/cache.go
type ConnectionCache struct {
    connections map[string]*ConnectionEntry  // key: machineID
    maxSize    int
    mu         sync.RWMutex
}

type ConnectionEntry struct {
    conn       *bindings.Connection  // From podman/pkg/bindings/connection.go
    userID     string                 // Current user (for reference)
    machineID  string                 // Machine ID
    refCount   int                    // Active request count
    lastUsed   time.Time              // Last activity timestamp
    isActive   bool                   // Currently serving a request
}

func NewConnectionCache(opts ConnectionCacheOptions) *ConnectionCache {
    cache := &ConnectionCache{
        connections: make(map[string]*ConnectionEntry),
        maxSize:    maxSize,
    }
    go cache.cleanupLoop()
    return cache
}

func (c *ConnectionCache) GetOrCreate(ctx context.Context, machineID, userID string, config MachineConfig) (*bindings.Connection, error)
func (c *ConnectionCache) Release(machineID string)
func (c *ConnectionCache) MarkActive(machineID string)
func (c *ConnectionCache) MarkIdle(machineID string)
func (c *ConnectionCache) cleanupLoop()
```

### Connection Manager

```go
// connections/manager.go
type ConnectionManager struct {
    cache           *ConnectionCache
    machineRepo     *MachineRepository
    encryption      *EncryptionService
    maxConcurrent   int
    queueTimeout    time.Duration
    connectionQueue chan connectionRequest
}

type connectionRequest struct {
    machineID string
    userID    string
    response  chan<- connectionResponse
}

type connectionResponse struct {
    conn *bindings.Connection
    err  error
}

func NewConnectionManager(
    cache *ConnectionCache,
    machineRepo *MachineRepository,
    encryption *EncryptionService,
    maxConcurrent int,
    queueTimeout time.Duration,
) *ConnectionManager

func (m *ConnectionManager) GetConnection(ctx context.Context, machineID, userID string) (*bindings.Connection, error)
func (m *ConnectionManager) QueueRequest(ctx context.Context, machineID, userID string) (*bindings.Connection, error)
func (m *ConnectionManager) Run()
```

### Connection Lifecycle

1. **Request arrives** for machine `X`
2. **Check cache** for existing connection to machine `X`
3. **If exists and idle**:
   - Mark as active
   - Increment reference count
   - Return connection
4. **If exists and active**:
   - Increment reference count
   - Return connection (shared)
5. **If not in cache**:
   - Check if at max concurrent limit
   - If at limit:
     - Find LRU idle connection, close it
     - Create new connection
   - Else if all connections active:
     - Queue request
     - Wait for timeout or connection to become available
   - Create new connection via forked Podman's `bindings.NewConnectionWithOptions`
   - Store in cache
   - Return connection
6. **Request completes**:
   - Decrement reference count
   - If ref count reaches 0: Mark as idle

---

## Authentication

### JWT Implementation

```go
// internal/auth/service.go
type AuthService struct {
    privateKey     *rsa.PrivateKey
    publicKey      *rsa.PublicKey
    expiration     time.Duration
    issuer         string
    userRepo       *UserRepository
    hasher         password.HashService
}

type JWTConfig struct {
    PrivateKeyPath string
    PublicKeyPath  string
    Expiration     time.Duration
    Issuer         string
}

func NewAuthService(config JWTConfig, userRepo *UserRepository) (*AuthService, error)
func (s *AuthService) GenerateToken(userID string) (string, error)
func (s *AuthService) ValidateToken(tokenString string) (*TokenClaims, error)

// TokenClaims represents the JWT claims
type TokenClaims struct {
    UserID string `json:"sub"`
    jwt.StandardClaims
}
```

### JWT Key Management

```go
// scripts/generate-keys.sh
#!/bin/bash
openssl genrsa -out jwt-private.pem 4096
openssl rsa -in jwt-private.pem -pubout -out jwt-public.pem
```

On startup, check if key files exist. If not, generate them:
```go
func loadOrGenerateKeys(privateKeyPath, publicKeyPath string) (*rsa.PrivateKey, *rsa.PublicKey, error)
```

### JWT Middleware

```go
// internal/middleware/auth.go
func JWTMiddleware(authService *AuthService) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Get token from Authorization: Bearer header
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                respondWithError(w, http.StatusUnauthorized, "TOKEN_MISSING", "Authentication required")
                return
            }
            
            // Extract token from Bearer scheme
            parts := strings.Split(authHeader, " ")
            if len(parts) != 2 || parts[0] != "Bearer" {
                respondWithError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Invalid token format")
                return
            }
            
            tokenString := parts[1]
            
            // Validate token
            claims, err := authService.ValidateToken(tokenString)
            if err != nil {
                respondWithError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Invalid token")
                return
            }
            
            // Attach user to context
            ctx := context.WithValue(r.Context(), authContextKey, claims.UserID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func GetUserIDFromContext(ctx context.Context) (string, error)
```

---

## API Endpoints

### Authentication

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/api/auth/login` | Authenticate user, return JWT in JSON response body | No |
| POST | `/api/auth/logout` | Client-side token removal | No |
| GET | `/api/auth/status` | Check if user is authenticated, return user info | No |

### Machines

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/api/machines` | Register new machine (with connection test) | Yes |
| GET | `/api/machines` | List all machines user has access to | Yes |
| GET | `/api/machines/{id}` | Get machine details | Yes |
| PUT | `/api/machines/{id}` | Update machine connection details | Yes |
| DELETE | `/api/machines/{id}` | Remove machine | Yes |
| GET | `/api/machines/{id}/access` | List users with access to machine | Yes |
| POST | `/api/machines/{id}/access` | Grant access to another user | Yes |
| DELETE | `/api/machines/{id}/access/{userId}` | Revoke access from user | Yes |

### Containers

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/api/machines/{id}/containers` | List containers on machine | Yes |
| POST | `/api/machines/{id}/containers` | Create container | Yes |
| GET | `/api/machines/{id}/containers/{containerId}` | Get container details | Yes |
| POST | `/api/machines/{id}/containers/{containerId}/start` | Start container | Yes |
| POST | `/api/machines/{id}/containers/{containerId}/stop` | Stop container | Yes |
| DELETE | `/api/machines/{id}/containers/{containerId}` | Remove container | Yes |

### Images

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/api/machines/{id}/images` | List images on machine | Yes |
| POST | `/api/machines/{id}/images/pull` | Pull image (streaming with progress) | Yes |
| DELETE | `/api/machines/{id}/images/{imageId}` | Remove image | Yes |

### Quadlets

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/api/machines/{id}/quadlets` | List all quadlets on machine | Yes |
| POST | `/api/machines/{id}/quadlets` | Create new quadlet | Yes |
| GET | `/api/machines/{id}/quadlets/{quadletId}` | Get quadlet details | Yes |
| PUT | `/api/machines/{id}/quadlets/{quadletId}` | Update quadlet content | Yes |
| DELETE | `/api/machines/{id}/quadlets/{quadletId}` | Remove quadlet | Yes |
| POST | `/api/machines/{id}/quadlets/{quadletId}/start` | Start quadlet | Yes |
| POST | `/api/machines/{id}/quadlets/{quadletId}/stop` | Stop quadlet | Yes |
| POST | `/api/machines/{id}/quadlets/{quadletId}/restart` | Restart quadlet | Yes |
| GET | `/api/machines/{id}/quadlets/{quadletId}/logs` | Get quadlet logs | Yes |
| POST | `/api/machines/{id}/quadlets/{quadletId}/generate` | Generate quadlet from resource | Yes |
| POST | `/api/machines/{id}/quadlets/import` | Import quadlets from directory | Yes |

### Secrets

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/api/machines/{id}/secrets` | List secret metadata on machine | Yes |
| GET | `/api/machines/{id}/secrets/{secretId}` | Inspect secret; `?reveal=true` (admin-only) includes the value | Yes |
| POST | `/api/machines/{id}/secrets` | Create secret (name + value; write level) | Yes |

See [description_secrets_api.md](description_secrets_api.md) for the full contract. Secret deletion is planned (slice 160-04); updates are a conscious non-goal (podman secrets are immutable).

### Events

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/api/machines/{id}/events` | SSE endpoint for Podman events | Yes |

---

## Error Handling

### Error Codes

```go
// internal/errors/errors.go
package errors

var (
    // Machines
    ErrMachineNotFound        = NewAppError("MACHINE_NOT_FOUND", "Machine not found", http.StatusNotFound)
    ErrMachineAlreadyExists   = NewAppError("MACHINE_ALREADY_EXISTS", "Machine with this name already exists", http.StatusBadRequest)
    ErrConnectionFailed       = NewAppError("CONNECTION_FAILED", "Failed to connect to machine", http.StatusBadRequest)
    ErrConnectionTestTimeout = NewAppError("CONNECTION_TEST_TIMEOUT", "Connection test timed out", http.StatusBadRequest)
    
    // Auth
    ErrInvalidCredentials = NewAppError("INVALID_CREDENTIALS", "Invalid username or password", http.StatusUnauthorized)
    ErrTokenExpired        = NewAppError("TOKEN_EXPIRED", "Token has expired", http.StatusUnauthorized)
    ErrTokenInvalid        = NewAppError("TOKEN_INVALID", "Invalid token", http.StatusUnauthorized)
    
    // Rate Limiting
    ErrRateLimitExceeded = NewAppError("RATE_LIMIT_EXCEEDED", "Too many requests", http.StatusTooManyRequests)
    
    // Containers
    ErrContainerNotFound        = NewAppError("CONTAINER_NOT_FOUND", "Container not found", http.StatusNotFound)
    ErrContainerAlreadyRunning   = NewAppError("CONTAINER_ALREADY_RUNNING", "Container is already running", http.StatusBadRequest)
    ErrContainerNotRunning       = NewAppError("CONTAINER_NOT_RUNNING", "Container is not running", http.StatusBadRequest)
    
    // Images
    ErrImageNotFound  = NewAppError("IMAGE_NOT_FOUND", "Image not found", http.StatusNotFound)
    ErrImagePullFailed = NewAppError("IMAGE_PULL_FAILED", "Failed to pull image", http.StatusInternalServerError)
    
    // Quadlets
    ErrQuadletNotFound         = NewAppError("QUADLET_NOT_FOUND", "Quadlet not found", http.StatusNotFound)
    ErrQuadletAlreadyExists    = NewAppError("QUADLET_ALREADY_EXISTS", "Quadlet with this name already exists", http.StatusConflict)
    ErrQuadletResourceRunning  = NewAppError("QUADLET_RESOURCE_RUNNING", "Quadlet-managed resource is running", http.StatusBadRequest)
    ErrQuadletAlreadyRunning    = NewAppError("QUADLET_ALREADY_RUNNING", "Quadlet is already running", http.StatusBadRequest)
    ErrQuadletNotRunning        = NewAppError("QUADLET_NOT_RUNNING", "Quadlet is not running", http.StatusBadRequest)
    
    // Secrets
    ErrSecretNotFound       = NewAppError("SECRET_NOT_FOUND", "Secret not found", http.StatusNotFound)
    ErrSecretAlreadyExists  = NewAppError("SECRET_ALREADY_EXISTS", "Secret with this name already exists", http.StatusConflict)
    ErrSecretInUse          = NewAppError("SECRET_IN_USE", "Secret is in use by one or more containers", http.StatusConflict)
    ErrSecretCreationFailed = NewAppError("SECRET_CREATION_FAILED", "Failed to create secret", http.StatusInternalServerError)
    
    // Generic
    ErrInternalError      = NewAppError("INTERNAL_ERROR", "An unexpected error occurred", http.StatusInternalServerError)
    ErrServiceUnavailable = NewAppError("SERVICE_UNAVAILABLE", "Service temporarily unavailable", http.StatusServiceUnavailable)
)

type AppError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Status  int    `json:"-"`
}

func NewAppError(code, message string, status int) *AppError
func (e *AppError) Error() string
```

### Error Response Format

```go
// internal/errors/handlers.go
type ErrorResponse struct {
    RequestID string       `json:"requestId"`
    Error     ErrorDetails `json:"error"`
}

type ErrorDetails struct {
    Message string `json:"message"`
    Code    string `json:"code"`
}

func RespondWithError(w http.ResponseWriter, err *AppError, requestID string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(err.Status)
    
    response := ErrorResponse{
        RequestID: requestID,
        Error: ErrorDetails{
            Message: err.Message,
            Code:    err.Code,
        },
    }
    
    // For 5xx errors, use generic message
    if err.Status >= 500 {
        response.Error.Message = "An unexpected error occurred"
        response.Error.Code = "INTERNAL_ERROR"
    }
    
    json.NewEncoder(w).Encode(response)
}
```

### Request ID Middleware

```go
// internal/middleware/requestid.go
func RequestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestID := uuid.New().String()
        ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func GetRequestID(ctx context.Context) string
```

---

## Rate Limiting

```go
// internal/middleware/ratelimit.go
type RateLimiter struct {
    store      *RateLimitStore
    requests   int
    window     time.Duration
}

type RateLimitStore struct {
    mu      sync.Mutex
    entries map[string]*RateLimitEntry
}

type RateLimitEntry struct {
    count     int
    windowStart time.Time
}

func NewRateLimiter(requests int, window time.Duration) *RateLimiter

func (rl *RateLimiter) Allow(userID string) bool

func RateLimitMiddleware(rl *RateLimiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            userID := GetUserIDFromContext(r.Context())
            if userID == "" {
                // Skip rate limiting for unauthenticated requests
                // (or apply IP-based rate limiting)
                next.ServeHTTP(w, r)
                return
            }
            
            if !rl.Allow(userID) {
                w.Header().Set("Content-Type", "application/json")
                w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.requests))
                w.Header().Set("X-RateLimit-Remaining", "0")
                w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(rl.window.Seconds()), 10))
                w.WriteHeader(http.StatusTooManyRequests)
                json.NewEncoder(w).Encode(ErrorResponse{
                    RequestID: GetRequestID(r.Context()),
                    Error: ErrorDetails{
                        Code:    "RATE_LIMIT_EXCEEDED",
                        Message: "Too many requests",
                    },
                })
                return
            }
            
            // Set rate limit headers
            remaining := rl.GetRemaining(userID)
            resetIn := rl.GetResetIn(userID)
            w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.requests))
            w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
            w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(resetIn.Seconds()), 10))
            
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## SSE Implementation

```go
// internal/domain/events/handler.go
func (h *EventHandler) HandleEvents(w http.ResponseWriter, r *http.Request) {
    machineID := chi.URLParam(r, "id")
    userID := GetUserIDFromContext(r.Context())
    
    // Verify user has access to machine
    if !h.machineService.UserHasAccess(userID, machineID) {
        respondWithError(w, http.StatusForbidden, "ACCESS_DENIED", "User does not have access to this machine")
        return
    }
    
    // Get or create connection to machine
    conn, err := h.connectionManager.GetConnection(r.Context(), machineID, userID)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "CONNECTION_FAILED", err.Error())
        return
    }
    
    // Set up SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    
    // Create event stream to Podman Service API
    req, err := conn.DoRequest(r.Context(), nil, http.MethodGet, "/events?stream=true", nil, nil)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "EVENT_STREAM_FAILED", err.Error())
        return
    }
    defer req.Body.Close()
    
    // Stream events from Podman to client
    reader := bufio.NewReader(req.Response.Body)
    for {
        select {
        case <-r.Context().Done():
            // Client disconnected
            return
        default:
            line, err := reader.ReadBytes(':')
            if err != nil {
                if err == io.EOF {
                    return
                }
                // Log error and continue
                h.logger.WithError(err).Error("Error reading from Podman event stream")
                continue
            }
            
            // Parse Podman event and forward to client
            // Podman SSE format: "event: <type>\ndata: <json>\n\n"
            w.Write(line)
            if f, ok := w.(http.Flusher); ok {
                f.Flush()
            }
        }
    }
}
```

---

## Progress Streaming

```go
// internal/domain/images/handler.go
func (h *ImageHandler) HandlePullImage(w http.ResponseWriter, r *http.Request) {
    machineID := chi.URLParam(r, "id")
    userID := GetUserIDFromContext(r.Context())
    
    // Parse request
    var req ImagePullRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondWithError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
        return
    }
    
    // Verify access
    if !h.machineService.UserHasAccess(userID, machineID) {
        respondWithError(w, http.StatusForbidden, "ACCESS_DENIED", "User does not have access to this machine")
        return
    }
    
    // Get connection
    conn, err := h.connectionManager.GetConnection(r.Context(), machineID, userID)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "CONNECTION_FAILED", err.Error())
        return
    }
    
    // Mark connection as active (prevent idle timeout)
    defer h.connectionManager MarkIdle(machineID)
    h.connectionManager.MarkActive(machineID)
    
    // Set up streaming response
    w.Header().Set("Content-Type", "application/json")
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming not supported", http.StatusInternalServerError)
        return
    }
    
    // Make request to Podman Service API
    podmanReq, err := conn.DoRequest(r.Context(), nil, http.MethodPost, 
        fmt.Sprintf("/images/pull?fromImage=%s", url.QueryEscape(req.Image)), nil, nil)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "PULL_REQUEST_FAILED", err.Error())
        return
    }
    defer podmanReq.Body.Close()
    
    // Generate operation ID
    operationID := uuid.New().String()
    
    // Stream response body to client with transformation
    reader := bufio.NewReader(podmanReq.Response.Body)
    for {
        line, err := reader.ReadBytes('\n')
        if err != nil {
            if err == io.EOF {
                // Send completion event
                writeJSONLine(w, flusher, OperationEvent{
                    OperationID: operationID,
                    Type:        "complete",
                    MachineID:   machineID,
                    State:       map[string]interface{}{"status": "completed"},
                    Timestamp:   time.Now().UTC().Format(time.RFC3339),
                })
                return
            }
            // Send error event
            writeJSONLine(w, flusher, OperationEvent{
                OperationID: operationID,
                Type:        "error",
                Error: &OperationError{
                    Code:    "STREAM_ERROR",
                    Message: err.Error(),
                },
            })
            return
        }
        
        // Parse Podman progress JSON
        var progress PodmanProgress
        if err := json.Unmarshal(line, &progress); err == nil {
            // Transform and send
            writeJSONLine(w, flusher, OperationEvent{
                OperationID: operationID,
                Type:        "progress",
                MachineID:   machineID,
                State: map[string]interface{}{
                    "current":   progress.Current,
                    "total":     progress.Total,
                    "status":    progress.Status,
                    "message":   progress.Message,
                },
                Timestamp: time.Now().UTC().Format(time.RFC3339),
            })
        }
    }
}

func writeJSONLine(w http.ResponseWriter, f http.Flusher, data interface{}) {
    jsonBytes, _ := json.Marshal(data)
    w.Write(append(jsonBytes, '\n'))
    f.Flush()
}
```

---

## Machine Registration Flow

```go
// internal/domain/machines/handler.go
func (h *MachineHandler) HandleCreateMachine(w http.ResponseWriter, r *http.Request) {
    userID := GetUserIDFromContext(r.Context())
    
    var req CreateMachineRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondWithError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
        return
    }
    
    // Validate required fields
    if req.Name == "" || req.Hostname == "" || req.SSHUsername == "" || req.SSHPrivateKey == "" {
        respondWithError(w, http.StatusBadRequest, "VALIDATION_FAILED", "Missing required fields")
        return
    }
    
    // Check name uniqueness for user
    if h.machineService.NameExistsForUser(userID, req.Name) {
        respondWithError(w, http.StatusBadRequest, "MACHINE_ALREADY_EXISTS", "Machine with this name already exists")
        return
    }
    
    // Validate SSH private key format
    if !isValidPEM(req.SSHPrivateKey) {
        respondWithError(w, http.StatusBadRequest, "INVALID_SSH_KEY", "Invalid SSH private key format")
        return
    }
    
    // Encrypt SSH private key
    encryptedKey, fingerprint, err := h.encryption.EncryptSSHKey(req.SSHPrivateKey)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "ENCRYPTION_FAILED", err.Error())
        return
    }
    
    // Test connection with timeout
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    config := bindings.Options{
        URI:      fmt.Sprintf("ssh://%s@%s:%d%s", req.SSHUsername, req.Hostname, req.Port, req.PodmanSocketPath),
        Identity: req.SSHPrivateKey,
    }
    
    _, err = bindings.NewConnectionWithOptions(ctx, config)
    if err != nil {
        respondWithError(w, http.StatusBadRequest, "CONNECTION_FAILED", fmt.Sprintf("Failed to connect to machine: %s", err.Error()))
        return
    }
    
    // Create machine
    machine := &models.Machine{
        ID:               uuid.New().String(),
        Name:             req.Name,
        OwnerID:          userID,
        Description:      req.Description,
        Hostname:         req.Hostname,
        Port:             req.Port,
        SSHUsername:      req.SSHUsername,
        SSHKeyEncrypted:  encryptedKey,
        SSHKeyFingerprint: fingerprint,
        PodmanSocketPath: req.PodmanSocketPath,
    }
    
    if err := h.machineRepo.Create(machine); err != nil {
        respondWithError(w, http.StatusInternalServerError, "CREATION_FAILED", err.Error())
        return
    }
    
    // Grant owner access
    access := &models.MachineAccess{
        MachineID:  machine.ID,
        UserID:     userID,
        Permission: models.PermissionAdmin,
    }
    if err := h.machineAccessRepo.Create(access); err != nil {
        respondWithError(w, http.StatusInternalServerError, "CREATION_FAILED", err.Error())
        return
    }
    
    // Return success
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(MachineResponse{FromModel(machine)})
}
```

---

## Logging

```go
// internal/config/config.go
type LogConfig struct {
    Level  string `mapstructure:"LOG_LEVEL"`
    Format string `mapstructure:"LOG_FORMAT"`
}

func NewLogger(config LogConfig) *logrus.Logger {
    logger := logrus.New()
    
    switch config.Level {
    case "debug":
        logger.SetLevel(logrus.DebugLevel)
    case "warn":
        logger.SetLevel(logrus.WarnLevel)
    case "error":
        logger.SetLevel(logrus.ErrorLevel)
    default:
        logger.SetLevel(logrus.InfoLevel)
    }
    
    switch config.Format {
    case "text":
        logger.SetFormatter(&logrus.TextFormatter{})
    default:
        logger.SetFormatter(&logrus.JSONFormatter{})
    }
    
    return logger
}

// Middleware for request logging
func LoggingMiddleware(logger *logrus.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            requestID := GetRequestID(r.Context())
            
            // Log request
            logger.WithFields(logrus.Fields{
                "requestId": requestID,
                "method":    r.Method,
                "path":      r.URL.Path,
                "remote":    r.RemoteAddr,
            }).Debug("Request started")
            
            // Wrap response writer to capture status
            wrapped := NewResponseWriter(w)
            
            next.ServeHTTP(wrapped, r)
            
            // Log response
            logger.WithFields(logrus.Fields{
                "requestId":   requestID,
                "status":      wrapped.status,
                "duration":    time.Since(start).Milliseconds(),
                "bytes":       wrapped.bytes,
            }).Info("Request completed")
        })
    }
}
```

---

## Encryption

```go
// internal/encryption/encryption.go
type EncryptionService struct {
    key []byte  // 32 bytes for AES-256
}

func NewEncryptionService(key string) (*EncryptionService, error)
func (s *EncryptionService) Encrypt(data string) ([]byte, error)
func (s *EncryptionService) Decrypt(data []byte) (string, error)
func (s *EncryptionService) EncryptSSHKey(privateKey string) ([]byte, string, error)
// Returns: encrypted key, fingerprint, error
```

---

## Server Startup

```go
// cmd/server/main.go
func main() {
    // Load configuration
    config, err := config.Load()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    
    // Initialize logger
    logger := NewLogger(config.Log)
    
    // Initialize database
    db, err := sql.Open("sqlite3", config.Database.Path)
    if err != nil {
        logger.Fatalf("Failed to open database: %v", err)
    }
    defer db.Close()
    
    // Run migrations
    if err := migrations.Run(db); err != nil {
        logger.Fatalf("Failed to run migrations: %v", err)
    }
    
    // Initialize encryption
    encryption := encryption.NewEncryptionService(config.Encryption.Key)
    
    // Initialize JWT (load or generate keys)
    authService, err := auth.NewAuthService(auth.JWTConfig{
        PrivateKeyPath: config.JWT.PrivateKeyPath,
        PublicKeyPath:  config.JWT.PublicKeyPath,
        Expiration:     config.JWT.Expiration,
        Issuer:         config.JWT.Issuer,
    }, userRepository)
    if err != nil {
        logger.Fatalf("Failed to initialize auth: %v", err)
    }
    
    // Initialize repositories
    userRepo := repositories.NewUserRepository(db)
    machineRepo := repositories.NewMachineRepository(db)
    accessRepo := repositories.NewMachineAccessRepository(db)
    
    // Initialize connection manager
    connectionCache := connections.NewConnectionCache(config.Connections.MaxConcurrent)
    connectionManager := connections.NewConnectionManager(
        connectionCache,
        machineRepo,
        encryption,
        config.Connections.MaxConcurrent,
        config.Connections.QueueTimeout,
    )
    go connectionManager.Run()
    
    // Initialize services
    machineService := services.NewMachineService(machineRepo, accessRepo, connectionManager, encryption)
    containerService := services.NewContainerService(connectionManager)
    imageService := services.NewImageService(connectionManager)
    quadletService := services.NewQuadletService(connectionManager)
    eventService := services.NewEventService(connectionManager)
    
    // Initialize handlers
    authHandler := handlers.NewAuthHandler(authService, userRepo)
    machineHandler := handlers.NewMachineHandler(machineService)
    containerHandler := handlers.NewContainerHandler(containerService, machineService)
    imageHandler := handlers.NewImageHandler(imageService, machineService)
    quadletHandler := handlers.NewQuadletHandler(quadletService, machineService)
    eventHandler := handlers.NewEventHandler(eventService, machineService)
    
    // Initialize middleware
    rateLimiter := middleware.NewRateLimiter(
        config.RateLimit.RequestsPerMinute,
        time.Minute,
    )
    
    // Create router
    router := chi.NewRouter()
    
    // Global middleware
    router.Use(middleware.RequestIDMiddleware)
    router.Use(middleware.LoggingMiddleware(logger))
    router.Use(middleware.Recoverer)
   router.Use(middleware.CORS(config.AllowedOrigins))
    
    // Routes
    router.Route("/api", func(r chi.Router) {
        // Auth
        r.Post("/auth/login", authHandler.Login)
        r.Post("/auth/logout", authHandler.Logout)
        r.Get("/auth/status", authHandler.Status)
        
        // Protected routes
        r.Group(func(r chi.Router) {
            r.Use(middleware.JWTMiddleware(authService))
            r.Use(middleware.RateLimitMiddleware(rateLimiter))
            
            // Machines
            r.Route("/machines", func(r chi.Router) {
                r.Post("/", machineHandler.Create)
                r.Get("/", machineHandler.List)
                r.Route("/{id}", func(r chi.Router) {
                    r.Get("/", machineHandler.Get)
                    r.Put("/", machineHandler.Update)
                    r.Delete("/", machineHandler.Delete)
                    r.Get("/access", machineHandler.ListAccess)
                    r.Post("/access", machineHandler.GrantAccess)
                    r.Delete("/access/{userId}", machineHandler.RevokeAccess)
                    
                    // Containers
                    r.Route("/containers", func(r chi.Router) {
                        r.Get("/", containerHandler.List)
                        r.Post("/", containerHandler.Create)
                        r.Route("/{containerId}", func(r chi.Router) {
                            r.Get("/", containerHandler.Get)
                            r.Post("/start", containerHandler.Start)
                            r.Post("/stop", containerHandler.Stop)
                            r.Delete("/", containerHandler.Delete)
                        })
                    })
                    
                    // Images
                    r.Route("/images", func(r chi.Router) {
                        r.Get("/", imageHandler.List)
                        r.Post("/pull", imageHandler.Pull)
                        r.Delete("/{imageId}", imageHandler.Delete)
                    })
                    
                    // Quadlets
                    r.Route("/quadlets", func(r chi.Router) {
                        r.Get("/", quadletHandler.List)
                        r.Post("/", quadletHandler.Create)
                        r.Post("/import", quadletHandler.Import)
                        r.Route("/{quadletId}", func(r chi.Router) {
                            r.Get("/", quadletHandler.Get)
                            r.Put("/", quadletHandler.Update)
                            r.Delete("/", quadletHandler.Delete)
                            r.Post("/start", quadletHandler.Start)
                            r.Post("/stop", quadletHandler.Stop)
                            r.Post("/restart", quadletHandler.Restart)
                            r.Get("/logs", quadletHandler.GetLogs)
                            r.Post("/generate", quadletHandler.Generate)
                        })
                    })
                    
                    // Events
                    r.Get("/events", eventHandler.events)
                })
            })
        })
    })
    
    // Start connection manager cleanup
    go connectionManager.Cleanup()
    
    // Start server
    logger.WithField("port", config.Port).Info("Starting server")
    if err := http.ListenAndServe(fmt.Sprintf(":%d", config.Port), router); err != nil {
        logger.Fatalf("Server failed: %v", err)
    }
}
```

---

## Docker Configuration

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /podland ./cmd/server

# Final stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Copy binary
COPY --from=builder /podland /app/podland

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Copy default config
COPY --from=builder /app/.env.example .env.example

# Create data directory
RUN mkdir -p /app/data

# Set permissions
RUN chown -R nobody:nogroup /app
USER nobody:nogroup

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# Run
CMD ["/app/podland"]
```

---

## Dependencies

```
# go.mod
module github.com/Homerus25/podland/backend

go 1.21

require (
    github.com/containers/podman/v6 v6.x.x  // Your forked version
    github.com/go-chi/chi/v5 v5.0.10
    github.com/go-playground/validator/v10 v10.16.0
    github.com/golang-jwt/jwt/v5 v5.0.0
    github.com/google/uuid v1.3.0
    github.com/kevinburke/ssh_config v1.2.0
    github.com/mattn/go-sqlite3 v1.14.17
    github.com/secure-systems-lab/go-securesystemslib v0.7.0
    github.com/sirupsen/logrus v1.9.3
    golang.org/x/crypto v0.14.0
)
```

---

## Testing

### Unit Tests
```go
// Example: machines/service_test.go
func TestMachineService_Create(t *testing.T) {
    // Setup
    mockRepo := &MockMachineRepository{}
    mockAccessRepo := &MockMachineAccessRepository{}
    mockConnManager := &MockConnectionManager{}
    mockEncryption := &MockEncryptionService{}
    
    service := NewMachineService(mockRepo, mockAccessRepo, mockConnManager, mockEncryption)
    
    // Test
    ctx := context.Background()
    req := &CreateMachineRequest{
        Name:        "test-machine",
        Hostname:    "localhost",
        SSHUsername: "user",
        SSHPrivateKey: "-----BEGIN RSA PRIVATE KEY-----",
    }
    
    // Mock connection test success
    mockConnManager.On("TestConnection", ctx, req).Return(nil)
    
    // Mock encryption
    mockEncryption.On("EncryptSSHKey", req.SSHPrivateKey).Return([]byte("encrypted"), "fingerprint", nil)
    
    // Mock repository
    mockRepo.On("Create", ctx, mock.Anything).Return(nil)
    mockAccessRepo.On("Create", ctx, mock.Anything).Return(nil)
    
    // Execute
    machine, err := service.Create(ctx, "user-123", req)
    
    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, machine)
    mockRepo.AssertExpectations(t)
}
```

### Integration Tests
```go
// tests/integration/machines_test.go
func TestMachineRegistration_Integration(t *testing.T) {
    // Setup test container with Podman
    ctx := context.Background()
    
    // Start Podman container for testing
    containerID, err := startTestPodmanContainer(ctx)
    assert.NoError(t, err)
    defer stopTestContainer(ctx, containerID)
    
    // Setup test database
    db := setupTestDB(t)
    defer teardownTestDB(db)
    
    // Create repositories and services
    machineRepo := repositories.NewMachineRepository(db)
    // ... setup other dependencies
    
    // Create test user
    userID := "test-user-123"
    // ... insert test user
    
    // Test machine registration
    req := &CreateMachineRequest{
        Name:           "test-machine",
        Hostname:       getContainerIP(containerID),
        Port:           22,
        SSHUsername:    "root",
        SSHPrivateKey:  testSSHKey,
        PodmanSocketPath: "/run/podman/podman.sock",
    }
    
    machine, err := machineService.Create(ctx, userID, req)
    assert.NoError(t, err)
    assert.NotNil(t, machine)
    
    // Verify connection works
    conn, err := connectionManager.GetConnection(ctx, machine.ID, userID)
    assert.NoError(t, err)
    assert.NotNil(t, conn)
    
    // Verify we can list containers
    containers, err := containerService.List(ctx, machine.ID, userID)
    assert.NoError(t, err)
    // ... assertions
}
```

---

## Open Questions for Backend

1. **Health Check Endpoint**: Should we add `/api/health` for Kubernetes/container orchestration?
2. **Prometheus Metrics**: Should we add `/api/metrics` endpoint for monitoring?
3. **OpenAPI/Swagger**: Should we generate OpenAPI specification for the API?
4. **API Versioning**: Should we add version prefix (e.g., `/api/v1/machines`)?

---

*Generated from comprehensive grilling sessions*
*Last updated: 2026-06-30*
