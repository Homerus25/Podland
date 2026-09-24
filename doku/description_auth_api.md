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
- Returns the JWT access token, the authenticated user's username, and the user's role in the response body
- Additionally sets the httpOnly refresh cookie (see POST /api/auth/refresh)
```json
{
  "token": "string (jwt)",
  "username": "string",
  "role": "string ('user' | 'admin')"
}
```

**Fields:**
- `role`: The authenticated user's role, read from the database (not a JWT claim), so role changes take effect immediately. `"user"` is the default; see GET /api/auth/status for details.

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
  "username": "string",
  "role": "string ('user' | 'admin')"
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

#### POST /api/setup/user

First-run bootstrap for a fresh instance. An instance with **zero
users** creates its first user here — always with the admin role — and
the caller is signed in on the spot: the response is the login payload
plus the httpOnly refresh cookie, exactly as at login. No Authorization
header; the endpoint is unauthenticated by design. Once any user
exists, the one-time offer is gone: every further call answers **409
`SETUP_COMPLETED`** with no side effects. The emptiness check and the
insert ride one SQLite write transaction, so of two concurrent setup
requests exactly one succeeds and the other receives the 409. Rate-
limited in the same per-IP auth group as login and refresh.

**Request:**
```json
Content-Type: application/json

{
  "username": "string",
  "password": "string"
}
```

**Response (Success - 200):**
- Same body as login; sets a fresh refresh cookie (Path=/api/auth)
```json
{
  "token": "string (jwt)",
  "username": "string",
  "role": "string ('admin')"
}
```

**Response (Error):**
- 409 Conflict: the instance already has a user (`SETUP_COMPLETED`)
- 400 Bad Request: password below the 8-character policy (`PASSWORD_TOO_SHORT`, "Password must be at least 8 characters"); malformed JSON body (`INVALID_REQUEST`, "Invalid request body"); missing or empty username/password (`VALIDATION_FAILED`, carrying the validator's message)

**Scripted setup (headless deployment):** the endpoint works with any
plain HTTP client — no special CLI tool is needed.

```bash
# 1. Probe: is this instance still fresh?
curl -s http://localhost:8080/api/auth/status
# -> {"authenticated":false,"needsSetup":true}

# 2. Create the first admin; save the access token from the response.
curl -s http://localhost:8080/api/setup/user \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"choose-a-strong-password"}'

# 3. The instance is configured: needsSetup is now false, and the
#    credentials log in at POST /api/auth/login as on any instance.
```

---

#### GET /api/auth/status

**Request:**
- Optional Authorization: Bearer header. Served without a token too
  (an invalid *presented* token still gets 401).

**Response (Success - 200):**
- Reports whether the caller is authenticated, and if so their ID and
  role; for anonymous callers it also reports whether the instance
  still needs first-run setup (users table empty, evaluated per
  request):
```json
{
  "authenticated": true,
  "userId": "string (uuid)",
  "role": "string ('user' | 'admin')",
  "needsSetup": false
}
```

**Fields:**
- `role`: The authenticated user's role. `"user"` is the default for all users; `"admin"` is granted to the first admin created through first-run setup (POST /api/setup/user). Role changes take effect immediately (the role is read from the database per request, not embedded in the JWT).
- `needsSetup`: `true` only for an **unauthenticated** caller on an instance whose users table is empty — the signal to route to the setup page (see POST /api/setup/user). It flips to `false` the moment setup completes. Authenticated callers always see `false`.

**Response (Error):**
- 401 Unauthorized: Invalid token (`TOKEN_INVALID`) — a *missing* token is not an error; the endpoint answers the anonymous shape above.

**Rate limiting:** login, refresh, this status probe, and the setup
endpoint share one per-IP budget (`RATE_LIMIT_AUTH_REQUESTS_PER_IP_PER_MINUTE`,
default 10/min). Exceeding it answers 429 `RATE_LIMIT_EXCEEDED`.

---

#### POST /api/auth/password

Self-service password change for the authenticated caller. Requires
Authorization: Bearer header with a valid token. The current password
proves the caller's authority; the new one must be at least 8
characters. On success the stored hash is replaced and **every** session
of the user is revoked — the caller receives a fresh login (new access
JWT in the body plus a new refresh cookie) so the device making the
change stays signed in; every other device is signed out and must log
in again with the new password. It lives under `/api/auth` because the
fresh refresh cookie shares the login/refresh cookie scope
(Path=/api/auth).

**Request:**
```json
Content-Type: application/json

{
  "currentPassword": "string",
  "newPassword": "string"
}
```

**Response (Success - 200):**
- Same body as login; sets a fresh refresh cookie
```json
{
  "token": "string (jwt)",
  "username": "string",
  "role": "string ('user' | 'admin')"
}
```

**Response (Error):**
- 401 Unauthorized: Wrong current password (`INVALID_CREDENTIALS`, message "Current password is incorrect"), or missing/invalid bearer token (`TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`)
- 400 Bad Request: New password shorter than 8 characters (`PASSWORD_TOO_SHORT`, "Password must be at least 8 characters"); malformed JSON body (`INVALID_REQUEST`, "Invalid request body"); missing or empty fields (`VALIDATION_FAILED`, carrying the validator's message)
- 404 Not Found: The token's user no longer exists (`USER_NOT_FOUND`)
