# Podman Management API: Users Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

---

## GET /api/users

Lists the Podland users. Available to **any authenticated caller**; the response shape depends on the caller's role — admins receive the full shape, every other authenticated caller a minimal shape. Anonymous requests are rejected.

**Request:**
```
Authorization: Bearer <token>
```

**Response (Success - 200):** a bare JSON array (no envelope wrapper).

Caller is an admin:
```json
[
  {
    "id": "string (uuid)",
    "username": "string",
    "role": "user" | "admin",
    "createdAt": "string (RFC 3339)"
  }
]
```

Caller is a regular user:
```json
[
  {
    "id": "string (uuid)",
    "username": "string"
  }
]
```

**Fields:**
- Entries are ordered by `username`.
- Admins receive exactly `id`, `username`, `role`, `createdAt` per user; regular users receive exactly `id`, `username`.
- The minimal shape exists so machine owners (not necessarily admins) can discover usernames when sharing a machine; role and creation metadata stay admin-only.
- The caller's role is resolved from the database on every request, so role changes take effect immediately.
- Password hashes never appear in any response.

**Response (Error):**
- 401 Unauthorized: Missing or invalid token (`TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`)
- 403 Forbidden: The caller's user record no longer exists (`ADMIN_REQUIRED`)
- 500 Internal Server Error: storage failure behind the lookup or the list query (generic `INTERNAL_ERROR`)

---

## POST /api/users

Creates a new user. **Admin-only**: the caller must authenticate with a valid JWT whose user has the `admin` role.

**Request:**
```
Content-Type: application/json
Authorization: Bearer <admin token>
```
```json
{
  "username": "string",
  "password": "string",
  "role": "user" | "admin"
}
```

**Fields:**
- `username`: Required, non-empty; must be unique.
- `password`: Required, non-empty; at least 8 characters (`PASSWORD_TOO_SHORT` otherwise).
- `role`: Optional. Omitted or empty creates a regular user; `"admin"` creates an administrator. Any other value is rejected (`VALIDATION_FAILED`). Only an admin can grant the admin role, and only through this endpoint.

**Response (Success - 201):**
```json
{
  "id": "string (uuid)",
  "username": "string",
  "role": "user" | "admin",
  "createdAt": "string (RFC 3339)"
}
```

**Fields:**
- `role`: The created user's role — the requested role, or `"user"` when the body omitted it.
- The password is stored bcrypt-hashed and never echoed; the response contains exactly `id`, `username`, `role`, `createdAt`.

The created user can immediately log in via `POST /api/auth/login` with the given credentials; the login response reports the new user's role.

**Response (Error):**
- 401 Unauthorized: Missing or invalid token (`TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`)
- 403 Forbidden: Authenticated caller without the admin role (`ADMIN_REQUIRED`)
- 400 Bad Request: Missing or empty username/password (`VALIDATION_FAILED`, "Missing required fields"); a `role` outside `"user"`/`"admin"` (`VALIDATION_FAILED`, `Role must be "user" or "admin"`); a password shorter than 8 characters (`PASSWORD_TOO_SHORT`, "Password must be at least 8 characters"); malformed JSON body (`INVALID_REQUEST`)
- 409 Conflict: Username already exists (`USERNAME_TAKEN`). A concurrent duplicate creation that races past the exists-check is also caught by the database's UNIQUE constraint and surfaces the same way.

---

## PUT /api/users/{id}/role

Changes a user's role. **Admin-only**: the caller must authenticate with a valid JWT whose user has the `admin` role.

**Request:**
```
Content-Type: application/json
Authorization: Bearer <admin token>
```
```json
{
  "role": "user" | "admin"
}
```

**Fields:**
- `role`: Required. The target user's new role; any other value than `"user"` or `"admin"` — an empty or omitted one included — is rejected (`VALIDATION_FAILED`).
- **Self-lockout guard:** an admin cannot change their own role (`SELF_ROLE_CHANGE`) — one misclick must never strip the instance of its administration. Editing the users table in the SQLite database directly remains the escape hatch for a genuinely intended self-demotion.
- The caller's role is irrelevant to the target: any existing user may be promoted or demoted, admins included.

**Response (Success - 200):** the updated user.
```json
{
  "id": "string (uuid)",
  "username": "string",
  "role": "user" | "admin",
  "createdAt": "string (RFC 3339)"
}
```

The change takes effect immediately: the admin gate, the users-list shape, and the auth status all resolve the role from the database per request, so existing access tokens of the target user gain or lose admin rights without re-login.

**Response (Error):**
- 401 Unauthorized: Missing or invalid token (`TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`)
- 403 Forbidden: Authenticated caller without the admin role (`ADMIN_REQUIRED`)
- 400 Bad Request: A `role` outside `"user"`/`"admin"`, empty, or missing (`VALIDATION_FAILED`, carrying the validator's message); the caller targeting their own role (`SELF_ROLE_CHANGE`, "Cannot change your own role"); malformed JSON body (`INVALID_REQUEST`)
- 404 Not Found: No user with the given `id` (`USER_NOT_FOUND`)
- 500 Internal Server Error: storage failure behind the lookup or the update (generic `INTERNAL_ERROR`)

---

## PUT /api/users/{id}/password

Resets a user's password. **Admin-only**: the caller must authenticate with a valid JWT whose user has the `admin` role. The caller must also re-authenticate with their own password: the request carries the acting admin's password, verified against the caller (the JWT subject) before the reset runs — a stolen or borrowed admin session alone cannot take over an account.

**Request:**
```
Content-Type: application/json
Authorization: Bearer <admin token>
```
```json
{
  "adminPassword": "string",
  "newPassword": "string"
}
```

**Fields:**
- `adminPassword`: Required. The **acting admin's own** password — named for the acting admin, since actor and target differ. Verified with the same bcrypt comparison the login and the self-service password change use. A wrong value answers 401 `INVALID_CREDENTIALS`, "Current password is incorrect" — the same message and code the self-service change produces.
- `newPassword`: Required; at least 8 characters (`PASSWORD_TOO_SHORT` otherwise).
- **Failure budget**: failed admin-password verifications count against the acting admin — five per fifteen minutes (the shared `RATE_LIMIT_AUTH_FAILURE_BUDGET` / `RATE_LIMIT_AUTH_FAILURE_WINDOW_MINUTES` pair; the users surface holds its own limiter instance keyed by the acting admin, separate from the auth surface's). Once spent, further resets answer 429 `RATE_LIMIT_EXCEEDED` with the standard `X-RateLimit-*` header trio **before any verification runs**, even with the correct admin password; only the window's expiry restores the budget. Successful resets and storage failures never consume it.
- **Self-lockout guard:** an admin cannot reset their **own** password through this endpoint (`SELF_PASSWORD_RESET`) — self-service `POST /api/auth/password` is the only self path. The guard runs before the password verification, so a doomed request burns neither a bcrypt compare nor failure budget, and it answers 400 regardless of the supplied password's correctness.

**Response (Success - 200):** the updated user.
```json
{
  "id": "string (uuid)",
  "username": "string",
  "role": "user" | "admin",
  "createdAt": "string (RFC 3339)"
}
```

**Fields:**
- The passwords are never echoed; the response contains exactly `id`, `username`, `role`, `createdAt`.

The reset takes effect immediately: **every session of the target user is revoked**, so all of the target's refresh tokens (stolen ones included) die with the old password. No fresh login is issued for the target — unlike the self-service `POST /api/auth/password` change, the caller is the admin, and the target must log in with the new password. The admin's own sessions are untouched. A target may be any existing user other than the caller.

**Response (Error):**
- 401 Unauthorized: Missing or invalid token (`TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`); a wrong `adminPassword` (`INVALID_CREDENTIALS`, "Current password is incorrect")
- 403 Forbidden: Authenticated caller without the admin role (`ADMIN_REQUIRED`)
- 400 Bad Request: A `newPassword` shorter than 8 characters (`PASSWORD_TOO_SHORT`, "Password must be at least 8 characters"); a missing or empty `adminPassword` or `newPassword` (`VALIDATION_FAILED`); the caller resetting their own password (`SELF_PASSWORD_RESET`, "Cannot reset your own password"); malformed JSON body (`INVALID_REQUEST`)
- 404 Not Found: No user with the given `id` (`USER_NOT_FOUND`; answered **before** the budget check, so an unknown target stays 404 unchanged even for a locked-out caller — and the lookup verifies nothing, so it never spends the budget)
- 429 Too Many Requests: The acting admin's credential-failure budget is spent (`RATE_LIMIT_EXCEEDED`, "Too many requests") — carries `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and `X-RateLimit-Reset` (seconds until the window resets)
- 500 Internal Server Error: storage failure behind the lookup, the update, or the session revocation (generic `INTERNAL_ERROR`)
