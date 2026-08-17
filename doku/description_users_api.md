# Podman Management API: Users Endpoints

Part of the [API contract](description_api.md). See the main document for base URL, common headers, and the error response format.

---

## POST /api/users

Creates a new user. **Admin-only**: the caller must authenticate with a valid JWT whose user has the `admin` role (the bootstrap user created by `cmd/initdb`).

**Request:**
```
Content-Type: application/json
Authorization: Bearer <admin token>
```
```json
{
  "username": "string",
  "password": "string"
}
```

There is deliberately no `role` field: this endpoint creates regular users only, so privilege escalation requires direct database access.

**Response (Success - 201):**
```json
{
  "id": "string (uuid)",
  "username": "string",
  "role": "user",
  "createdAt": "string (RFC 3339)"
}
```

**Fields:**
- `role`: Always `"user"` for endpoint-created users.
- The password is stored bcrypt-hashed and never echoed; the response contains exactly `id`, `username`, `role`, `createdAt`.

The created user can immediately log in via `POST /api/auth/login` with the given credentials.

**Response (Error):**
- 401 Unauthorized: Missing or invalid token (`TOKEN_INVALID`, `TOKEN_EXPIRED`)
- 403 Forbidden: Authenticated caller without the admin role (`ADMIN_REQUIRED`)
- 400 Bad Request: Missing or empty username/password (`VALIDATION_FAILED`); malformed JSON body (`INVALID_REQUEST`)
- 409 Conflict: Username already exists (`USERNAME_TAKEN`). A concurrent duplicate creation that races past the exists-check is also caught by the database's UNIQUE constraint and surfaces the same way.
