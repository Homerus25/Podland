---
title: "Own the 401: one response interceptor (architecture review C14)"
parent: none
date: 2026-09-15
status: open
labels:
  - needs-triage
  - frontend
  - refactoring
author: vibe-agent
---

## Problem Statement

CONTEXT.md promises a "global 401 interceptor for token expiration", but the
response side of auth has no owner. The access token expires mid-session and
every widget shows its own error alert after one doomed React Query retry
(`retry: 1` retries even 400s); refresh only happens at mount via
`AuthContext`'s silent resolver. The NDJSON stream transport and file
downloads use plain fetch and are invisible to any axios interceptor, so a
mid-session expiry during a prune/auto-update/logs stream dies as a
per-widget error too. `authApi` rides the same intercepted axios instance
the policy would live on — a recursion trap, made sharper by the backend
returning 401 `INVALID_CREDENTIALS` for a wrong password, which is not
session death. The design session also confirmed the query cache survives
logout, so a different user logging in next can be served the previous
user's cached lists.

## Solution

One 401 policy in `shared/utils/api`, two transports. A response
interceptor on the shared axios instance owns 401s: it runs a single-flight
refresh on a bare, interceptor-free transport and replays the failed
request once with the new token. The stream transport routes its 401s
through the same exported policy and re-fetches once. Any refresh failure —
auth rejection or network — or a 401 on a replayed request is session
death: a registered session-expired seam fires exactly once and `AuthContext`
runs the one existing exit path (`logout()`, which also clears the query
cache), landing the user on `/login` with their deep link preserved. The
React Query retry predicate stops retrying deterministic 4xx. 401 behavior
is pinned once in shared suites; feature suites drop their 401 stories.
File downloads stay outside the policy by decision.

## Decisions (from the design session — do not relitigate)

1. **Scope**: the policy covers the shared axios instance and the NDJSON
   stream transport. `download.ts` is excluded — a dead download link is
   low-stakes. A 401 means the request was rejected before execution, so
   replaying (including streamed POSTs) is safe.
2. **Bare transport**: `shared/utils/api` creates and exports one
   interceptor-free axios instance (same configured base URL). The policy's
   refresh POST and `authApi`'s login/refresh/logout both ride it, so no
   module cycle exists and auth calls are structurally outside the 401
   policy. The backend keeps `/auth/login`, `/auth/refresh`, and
   `/auth/logout` unauthenticated/cookie-based, so auth calls never need
   the bearer — and login's 401 `INVALID_CREDENTIALS` never reaches the
   policy.
3. **Policy shape**: one exported `handleUnauthorized(): Promise<boolean>`.
   `true` = a fresh token was installed, the caller retries its request
   once. `false` = the session is dead, the seam has already fired, the
   caller surfaces the error. One policy, two transports.
4. **Single-flight**: one shared in-flight refresh promise. N concurrent
   401s (typical dashboard load) trigger exactly one `POST /auth/refresh`;
   all queued callers replay with the new token; latecomers after
   resolution use it naturally.
5. **Replay once**: each transport marks its replayed request (axios
   config flag; stream transport loop state). A 401 on a marked request
   goes straight to session death — no second refresh. (Comparing the
   bearer against the last-refreshed token instead misclassifies a late
   401 from an old-token request as death — rejected.)
6. **Seam**: a module-level register/unregister session-expired handler in
   `shared/utils/api`, symmetric with `setBearerToken`. Firing goes through
   an idempotent once-latch that resets whenever a new token is installed,
   so both death paths (refresh rejected, replay rejected) fire exactly
   once per session death. Firing with no handler registered is a no-op,
   so the policy's unit tests need no `AuthContext`.
7. **Failure classification**: any refresh failure — auth rejection or
   network error — is session death. This matches the mount-time
   `resolveFromRefreshSession` catch-all; the alternative (only auth
   rejections log out, network blips self-heal) leaves a zombie state of
   erroring widgets under a "logged in" user and doubles the test matrix.
8. **Wiring**: `AuthProvider` registers the seam on mount, unregisters on
   unmount; the handler runs the existing `logout()` (best-effort revoke
   plus local clear). Redirect is silent via `ProtectedRoute` — no
   "session expired" notice; CONTEXT.md's promise is only the redirect.
9. **Cache**: `logout()` clears the query cache via `useQueryClient()`
   (`AuthProvider` sits inside `QueryClientProvider`), in its `finally`
   next to the existing state drop. No clear on login — the cache is
   in-memory and `logout()` is the only in-session identity exit.
10. **Retry predicate**: retry once only for network errors or 5xx; never
    any 4xx. 401s will already have been refreshed and replayed by the
    policy.
11. **Testing**: mechanics pinned in shared suites only —
    `shared/utils/api` unit tests (refresh-replay success; refresh
    rejected → seam exactly once + original 401 to the caller; concurrent
    401s → exactly one refresh; login 401 on the bare transport → no
    refresh, no seam; replayed 401 → seam once, no second refresh),
    stream-transport MSW tests (refresh → re-fetch → values pumped;
    rejected → seam + envelope; abort during refresh propagates), and one
    App-level wiring test (mid-query expiry lands on `/login`). Feature
    suites drop 401 stories — `MachinesPage`'s per-widget alert story is
    deleted, not replaced.
12. **Docs**: one line in `docs/frontend-conventions.md` naming the owner.
    `CONTEXT.md` already settled this decision, and no endpoint contract
    changes, so neither it nor `doku/` need edits.

## Slices

- `420-401-response-interceptor-01-bare-auth-transport` — the
  interceptor-free transport exported from `shared/utils/api`;
  `authApi` moves onto it. Pure prefactor, zero behavior change.
- `420-401-response-interceptor-02-interceptor-policy` — the response
  interceptor, `handleUnauthorized` single-flight refresh with replay-once,
  the once-latch seam, the 5xx/network-only retry predicate, the shared
  unit stories, and the MachinesPage 401 story deletion.
- `420-401-response-interceptor-03-authcontext-wiring` — the seam wired to
  `logout()`, cache clear on logout, App-level session-death test.
- `420-401-response-interceptor-04-stream-coverage` — `streamNdjson`
  routes 401s through the policy (refresh → re-fetch once, else envelope),
  transport-level MSW tests.
- `420-401-response-interceptor-05-conventions-update` — the conventions
  line naming the 401 owner and forbidding feature-suite 401 stories.
