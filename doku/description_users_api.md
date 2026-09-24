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

Resets a user's password. **Admin-only**: the caller must authenticate with a valid JWT whose user has the `admin` role.

**Request:**
```
Content-Type: application/json
Authorization: Bearer <admin token>
```
```json
{
  "newPassword": "string"
}
```

**Fields:**
- `newPassword`: Required; at least 8 characters (`PASSWORD_TOO_SHORT` otherwise). There is no current-password check — the caller is the admin, not the target.

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
- The password is stored bcrypt-hashed and never echoed; the response contains exactly `id`, `username`, `role`, `createdAt`.

The reset takes effect immediately: **every session of the target user is revoked**, so all of the target's refresh tokens (stolen ones included) die with the old password. No fresh login is issued for the target — unlike the self-service `POST /api/auth/password` change, the caller is the admin, and the target must log in with the new password. For any other target the admin's own sessions are untouched; resetting their **own** password through this endpoint revokes the admin's sessions like any other target's, current device included. A target may be any existing user.

**Response (Error):**
- 401 Unauthorized: Missing or invalid token (`TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`)
- 403 Forbidden: Authenticated caller without the admin role (`ADMIN_REQUIRED`)
- 400 Bad Request: A password shorter than 8 characters (`PASSWORD_TOO_SHORT`, "Password must be at least 8 characters"); a missing or empty `newPassword` (`VALIDATION_FAILED`); malformed JSON body (`INVALID_REQUEST`)
- 404 Not Found: No user with the given `id` (`USER_NOT_FOUND`)
- 500 Internal Server Error: storage failure behind the lookup, the update, or the session revocation (generic `INTERNAL_ERROR`)
