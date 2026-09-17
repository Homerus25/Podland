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
- Returns the JWT access token and the authenticated user's username in the response body
- Additionally sets the httpOnly refresh cookie (see POST /api/auth/refresh)
```json
{
  "token": "string (jwt)",
  "username": "string"
}
```

**Response (Error):**
- 401 Unauthorized: Invalid credentials (`INVALID_CREDENTIALS`)
- 400 Bad Request: Malformed JSON body (`INVALID_REQUEST`, "Invalid request body"); missing or empty username/password (`VALIDATION_FAILED`, carrying the validator's message)

---

#### POST /api/auth/refresh

Renews an access JWT from the httpOnly refresh cookie set by login (or a
previous refresh). No Authorization header; the cookie is the credential.
Rotation is the only refresh path: every successful call replaces the
session row behind the cookie with a fresh token (and renewed, sliding
expiry) and sets the rotated cookie.

**Request:**
- No body; sends the `refresh_token` httpOnly cookie (Path=/api/auth)

**Response (Success - 200):**
- Same body as login; sets a rotated refresh cookie
```json
{
  "token": "string (jwt)",
  "username": "string"
}
```

**Response (Error):**
- 401 Unauthorized: Missing/unknown/replayed token (`AUTH_REFRESH_INVALID`) or expired session (`AUTH_SESSION_EXPIRED`)
- Rate-limited like login

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
