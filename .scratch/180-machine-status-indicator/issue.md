---
title: Machine status indicator on the machines overview
parent: none
date: 2026-08-29
status: ready-for-agent
labels:
  - ready-for-agent
  - machines
  - backend
  - frontend
  - feature
author: vibe-agent
---

## Problem Statement

The machines overview page shows every registered machine the same way, whether the host is reachable or not. A user whose fleet contains a downed machine only finds out after navigating into it and waiting for a connection error to surface. There is no at-a-glance answer to the most basic question about a fleet: which machines are currently usable?

## Solution

Each machine card on the overview page gets a color-coded status indicator in its top-right corner: green "online" when the machine's Podman service answers over SSH, red "offline" when it does not. The status comes from a new live-check endpoint per machine, queried by the frontend once per card when the page is opened. Offline is a valid answer, not an error: the check is fast for reachable machines, bounded for dead ones, and the list endpoint itself stays untouched and instant.

## User Stories

1. As a machine owner, I want to see at a glance which of my machines are online, so that I don't waste time opening detail pages for dead hosts.
2. As a machine owner, I want offline machines shown in red, so that I can immediately spot failures in my fleet.
3. As a shared user with read access, I want to see a machine's status, so that I know whether it is usable before navigating into it.
4. As a user, I want a gray "checking" indicator while the status request is in flight, so that I don't mistake an in-flight check for a dead machine.
5. As a user, I want statuses re-checked every time I open the machines overview, so that the indicators reflect reality after I fix or reboot a machine.
6. As a user, I want the last-checked time available on the indicator, so that I can judge how fresh the status is.
7. As a colorblind user, I want a text label next to the color, so that the indicator is readable without relying on color alone.
8. As a user, I want offline to be a successful API response, so that the indicator and any tooling treat offline as information rather than failure.
9. As a user, I want the machines list to load instantly regardless of how many machines are offline, so that the overview is never blocked by slow hosts (status fills in per card, independently).
10. As a machine owner, I want status checks to reuse established connections when available, so that repeated checks on reachable machines are cheap.
11. As an admin, I want the detailed reason for a failed check logged server-side, so that I can diagnose why a machine reports offline.
12. As a user, I want connection error details kept out of the status response, so that infrastructure details of machines shared with me are not leaked.
13. As a user without access to a machine, I want status requests for it rejected, so that machine existence is not exposed to me.
14. As a user requesting the status of a nonexistent machine, I want a 404, so that the endpoint behaves consistently with the rest of the machine subtree.
15. As a user navigating away and back to the overview, I want the checks to re-run rather than serve cached results, so that I never see stale colors.
16. As a user, I want a gray "unknown" indicator if the status request itself fails, so that the card communicates uncertainty instead of lying with a color.
17. As a developer, I want the probe bounded by a timeout, so that unreachable hosts cannot hang requests indefinitely.

## Implementation Decisions

Rulings from the grilling session on 2026-08-29:

- **Status semantics.** "online" means the SSH connection to the host succeeds AND the Podman service API answers its ping over that connection. Anything else — SSH dial failure, auth failure, missing socket, ping failure, or timeout — is "offline". There is no third server-side state; partial reachability is not distinguished.
- **New endpoint.** `GET /api/machines/{id}/status`, gated by JWT plus the existing machine-access middleware at read level. Response is always HTTP 200 with `{"status": "online" | "offline", "checkedAt": "<RFC3339>"}` when access is granted — offline is a valid answer, not an error. 404 for unknown machine, 403 for users without access, matching the existing get-machine behavior.
- **No status in the list endpoint.** The initial request asked for status in the list-machines response; this was explicitly dropped during grilling. The frontend queries the status endpoint per card instead. Rationale: a status field in the list would force either blocking checks (list latency becomes the slowest dead machine) or a server-side status cache with refresh triggers (rejected complexity). The list endpoint stays unchanged.
- **Probe implementation.** The machines connection interface gains one new method: a live probe for an already-persisted machine that reuses the cached per-machine connection when present (paying a full SSH dial only when cold) and pings the Podman service API through it. The probe is bounded by the same timeout regime as the registration connection test (10 seconds default, env-configurable). On any failure the machine is offline.
- **No failure details in the response.** The detailed error (host unreachable, auth rejected, socket missing) is logged server-side with the requestId. The response payload contains only status and checkedAt, since machines are shared and SSH-level errors describe the owner's infrastructure.
- **Frontend data flow.** A per-card status hook backed by React Query, keyed per machine, with stale time zero so every mount of the overview triggers a fresh check. No polling, no refetch interval — status refreshes happen when the page is (re)opened, which also keeps the app's no-polling precedent intact.
- **Indicator UI.** A compact Chakra Badge with a colored dot plus text label, placed in the card header next to the permission badge (which already occupies the top-right corner; the status badge sits to its left). States: green "online", red "offline", gray "checking" while the request is in flight, gray "unknown" if the request itself errors. The text label keeps it colorblind-safe. The checkedAt time is surfaced as a tooltip on the badge.
- **Permission level.** Read access suffices. Listing containers on a shared machine already opens SSH connections at read level; status is the same class of operation. Write/admin gating would leave shared read users with permanently gray cards.

## Testing Decisions

Good tests assert externally visible behavior only — HTTP status codes, payload shape, rendered DOM — never how the probe dials or which internal path produced a result.

- **Backend service tests** at the existing connection-manager seam (the mocked interface in the machines domain): probe success maps to online, probe error maps to offline, unknown machine maps to not-found, missing access maps to denied. Prior art: the machine service tests covering the registration connection test, including its timeout case.
- **Backend handler tests** at the HTTP level: 200 with status/checkedAt for both online and offline, 404, 403, read-level access accepted. Prior art: the secrets and quadlets handler tests; this adds the first handler test to the machines domain using that established pattern.
- **Frontend tests** with MSW mocking the status endpoint per machine: online and offline badges render with the right colors and text, "checking" shows while pending, "unknown" shows when the request errors, and the checkedAt tooltip is present. Prior art: the machines overview page test and the state badge tests.
- **No new seams.** The only seam change is one added method on the existing connection-manager interface in the machines domain, already backed by a mock in the shared test utilities.

## Out of Scope

- Server-Sent Events push of status changes (mount-triggered re-checks cover the need).
- Persisting last-seen or uptime history in the database.
- Any server-side caching of status results.
- Status in the list-machines response (explicitly dropped during grilling).
- Polling or auto-refresh while the page is open.
- A manual refresh button on the overview.
- Coarse failure reasons in the status response.
- A status indicator on the machine detail page.

## Further Notes

- A status check cold-starts an SSH connection that then sits in the shared connection cache until evicted; opening the overview primes connections for all visible machines. This side effect is accepted — the cache's LRU eviction and concurrency limits already bound it.
- A probe timeout is reported as offline, consistent with the fail-fast posture toward dead hosts.
- The API documentation for the machine subtree should gain the new endpoint's contract alongside implementation.
