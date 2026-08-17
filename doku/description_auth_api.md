# Podman Management API: Auth Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

---

## Authentication

### Mechanism
- **JWT tokens** signed with RS256
- **Transport**: Authorization: Bearer header
- **Token expiration**: Configurable, default 60 minutes

### Auth Endpoints

#### POST /api/auth/login

**Request:**
```json
Content-Type: application/json

{
  "username": "string",
  "password": "string"
}
```

**Response (Success - 200):**
- Returns JWT token in response body
- Returns user information
```json
{
  "token": "string (jwt)",
  "user": {
    "id": "string (uuid)",
    "username": "string"
  }
}
```

**Response (Error):**
- 401 Unauthorized: Invalid credentials (`INVALID_CREDENTIALS`)
- 400 Bad Request: Invalid request format

---

#### POST /api/auth/logout

**Request:**
- No body required
- Client should remove stored token

**Response (Success - 200):**
```json
{
  "message": "Logged out successfully"
}
```

---

#### GET /api/auth/status

**Request:**
- Requires Authorization: Bearer header with valid token

**Response (Success - 200):**
- Reports whether the caller is authenticated, and if so their ID and role
- Served without a token too: `{"authenticated": false}` with 200 (an invalid *presented* token still gets 401)
```json
{
  "authenticated": true,
  "userId": "string (uuid)",
  "role": "string ('user' | 'admin')"
}
```

**Fields:**
- `role`: The authenticated user's role. `"user"` is the default for all users; `"admin"` is granted to the bootstrap user created by `cmd/initdb`. Role changes take effect immediately (the role is read from the database per request, not embedded in the JWT).

**Response (Error):**
- 401 Unauthorized: Invalid or missing token (`TOKEN_MISSING` or `TOKEN_INVALID`)
