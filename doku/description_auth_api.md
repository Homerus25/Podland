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
  "password": "string",
  "rememberDevice": false
}
```

**Fields:**
- `rememberDevice` (optional, default `false`): the remember-me opt-in. `true` makes the login persistent — the session row is flagged remembered and lives on the remembered TTL (`SESSION_REMEMBER_TTL_DAYS`, default 90 days, sliding on every refresh), and the refresh cookie carries a matching `Max-Age` so it survives closing the browser. Absent or `false` keeps today's behavior exactly: a browser-session refresh cookie and the `SESSION_TTL_DAYS` session TTL.

**Response (Success - 200):**
- Returns the JWT access token, the authenticated user's username, and the user's role in the response body
- Additionally sets the httpOnly refresh cookie (see POST /api/auth/refresh). With `rememberDevice: true` the cookie carries `Max-Age` equal to the full remembered TTL; otherwise it has no `Max-Age` (a browser-session cookie)
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
- 429 Too Many Requests: credential-failure budget exhausted (`RATE_LIMIT_EXCEEDED`) — see [Credential-failure budget](#credential-failure-budget)

**Credential-failure budget:** failed logins — wrong password and
unknown username alike — count against the username in the request
body. After five failures within fifteen minutes
(`RATE_LIMIT_AUTH_FAILURE_BUDGET` / `RATE_LIMIT_AUTH_FAILURE_WINDOW_MINUTES`),
further fresh logins for that name answer 429 `RATE_LIMIT_EXCEEDED`
**even with the correct password**, and only the window's expiry
restores access. The lockout bars fresh logins only: an active refresh
session (POST /api/auth/refresh) keeps working while its username is
locked out. Successful logins never consume the budget. Rejections
carry the same `X-RateLimit-Limit` / `X-RateLimit-Remaining` /
`X-RateLimit-Reset` headers the per-IP cap sets.

---

#### POST /api/auth/refresh

Renews an access JWT from the httpOnly refresh cookie set by login (or a
previous refresh). No Authorization header; the cookie is the credential.
Rotation is the only refresh path: every successful call replaces the
session row behind the cookie with a fresh token (and renewed, sliding
expiry) and sets the rotated cookie. A remembered session (created by a
`rememberDevice: true` login) keeps its remembered flag across the
rotation, renews on the remembered TTL, and the rotated cookie re-sets
its `Max-Age` to the full remembered TTL — the cookie slides in lockstep
with the session row. An unremembered session keeps today's behavior:
the `SESSION_TTL_DAYS` sliding renewal and a cookie with no `Max-Age`.

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
- Same body as login; sets a fresh refresh cookie (Path=/api/auth). The session is always unremembered — no `Max-Age`, exactly as a login without the `rememberDevice` flag; the bootstrap stays simple, and the user can log out and back in with the box checked
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
default 10/min). Exceeding it answers 429 `RATE_LIMIT_EXCEEDED`. Login
additionally spends the shared [credential-failure budget](#credential-failure-budget)
keyed per username.

---

### Credential-failure budget

Login and the self-service password change share one brake: a count of
failed credential verifications per identity within a fixed window,
configured as one pair — `RATE_LIMIT_AUTH_FAILURE_BUDGET` (default 5)
and `RATE_LIMIT_AUTH_FAILURE_WINDOW_MINUTES` (default 15).

- **Login** keys the budget on the username in the request body; a
  wrong password and an unknown username spend it alike. Once spent,
  further fresh logins for that name answer 429
  `RATE_LIMIT_EXCEEDED` with `X-RateLimit-Limit` /
  `X-RateLimit-Remaining` (`0`) / `X-RateLimit-Reset` headers, even
  with the correct password. Only the window's expiry restores access;
  an active refresh session keeps working throughout.
- **Password change** (POST /api/auth/password) keys the same budget on
  the caller: wrong current-password attempts spend it; once spent,
  further attempts answer 429 `RATE_LIMIT_EXCEEDED` until the window
  expires. A single wrong attempt still returns the unchanged 401.
- **Only failures spend the budget.** Successful logins and password
  changes, storage failures, and validation failures never do.
- The per-IP auth cap (`RATE_LIMIT_AUTH_REQUESTS_PER_IP_PER_MINUTE`)
  stays layered underneath for the unauthenticated routes; whichever
  budget rejects first writes its own headers on the 429.

---

#### POST /api/auth/password

Self-service password change for the authenticated caller. Requires
Authorization: Bearer header with a valid token. The current password
proves the caller's authority; the new one must be at least 8
characters. On success the stored hash is replaced and **every** session
of the user is revoked — remembered ones included, so a compromised
password cannot be ridden by remembered devices — and the caller
receives a fresh login (new access JWT in the body plus a new refresh
cookie) so the device making the change stays signed in; every other
device is signed out and must log in again with the new password. The
fresh session is unremembered (no cookie `Max-Age`). It lives under
`/api/auth` because the fresh refresh cookie shares the login/refresh
cookie scope (Path=/api/auth).

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
- 429 Too Many Requests: credential-failure budget exhausted (`RATE_LIMIT_EXCEEDED`) — wrong current-password attempts count against the caller into the shared budget; see [Credential-failure budget](#credential-failure-budget)
