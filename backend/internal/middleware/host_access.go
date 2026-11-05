package middleware

import (
	"errors"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/permission"
)

// hostRepoGetter is the slice of the host repository
// RequireHostAccess needs: resolving a host ID. Declared locally
// (rather than importing the domain interface) so the middleware
// depends only on the two lookups it actually performs; the concrete
// *models.HostRepository and the test mocks satisfy it structurally.
type hostRepoGetter interface {
	GetByID(id string) (*models.Host, error)
}

// accessRepoChecker is the slice of the host-access repository
// RequireHostAccess needs: the boolean access probe shared with
// HostService.UserHasAccess.
type accessRepoChecker interface {
	HasAccess(hostID, userID string) (bool, error)
}

// resolveHostAndCaller performs the checks RequireHostAccess and
// RequireHostPermission share: the request must carry an
// authenticated user ID (401 otherwise, RequireAdmin's contract for a
// miswired route), a host ID URL param (400), and a known host
// (404 HOST_NOT_FOUND, the hosts domain's answer, uniformly
// across the subtree). On any failure it has already written the error
// response and returns ok=false; on success it returns the host and
// user IDs the access probe needs.
func resolveHostAndCaller(w http.ResponseWriter, r *http.Request, hostRepo hostRepoGetter) (hostID, userID string, ok bool) {
	requestID := contextutils.GetRequestID(r.Context())
	userID = contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		apperrors.RespondWithError(w, apperrors.ErrTokenInvalid, requestID)
		return "", "", false
	}

	hostID = r.PathValue("id")
	if hostID == "" {
		apperrors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_HOST_ID", "Host ID is required", requestID)
		return "", "", false
	}

	host, err := hostRepo.GetByID(hostID)
	if err != nil {
		apperrors.RespondWithError(w, apperrors.ErrInternalError, requestID)
		return "", "", false
	}
	if host == nil {
		apperrors.RespondWithError(w, apperrors.ErrHostNotFound, requestID)
		return "", "", false
	}

	return hostID, userID, true
}

// RequireHostAccess gates a route behind the caller's host_access
// row. It must run after JWTMiddleware (it reads the authenticated user
// ID from the request context) and is designed for the
// /api/hosts/{id} subtree, taking the host ID from the chi URL
// parameter — so one seam covers the hosts endpoints and every
// downstream family that rides on a host (containers, images,
// quadlets, events) including ones added later.
//
// Semantics, mirroring the hosts domain (HostService.
// GetHostByID / Delete):
//
//   - Unknown host ID → 404 HOST_NOT_FOUND, the same answer the
//     hosts endpoints give, uniformly across the subtree (the
//     downstream services used to surface this as 500 or 502).
//   - Known host, no access row → 403 ACCESS_DENIED. Access is
//     resolved per request from the database — like RequireAdmin's role
//     lookup — so grants and revocations take effect immediately
//     rather than at token expiry.
//   - A caller that is authenticated but user-ID-less surfaces as 401,
//     matching RequireAdmin: an unauthenticated caller, not an
//     authorization decision.
//   - A failing lookup gets 500 rather than a silent allow/deny guess.
//
// The hosts handlers keep their own service-level checks; this
// middleware is the outer gate that extends the same contract to the
// downstream endpoints, which previously trusted the JWT alone.
func RequireHostAccess(
	hostRepo hostRepoGetter,
	accessRepo accessRepoChecker,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hostID, userID, ok := resolveHostAndCaller(w, r, hostRepo)
			if !ok {
				return
			}

			hasAccess, err := accessRepo.HasAccess(hostID, userID)
			if err != nil {
				apperrors.RespondWithError(w, apperrors.ErrInternalError, contextutils.GetRequestID(r.Context()))
				return
			}
			if !hasAccess {
				apperrors.RespondWithError(w, apperrors.ErrAccessDenied, contextutils.GetRequestID(r.Context()))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireHostPermission gates a route behind the LEVEL of the
// caller's host_access row rather than its mere existence. Use it
// inside the RequireHostAccess-protected /api/hosts/{id} subtree
// on mutating routes: RequireHostAccess stays the subtree-level
// read gate, this adds "the stored permission must be at least
// `required`" for endpoints that change remote state — container
// create/delete/start/stop, image pull/delete, quadlet
// create/import/update/delete/start/stop/restart, and later secret
// writes. The level comparison itself is owned by
// permission.RequireLevel (lookup error ⇒ 500, level below required ⇒
// 403, never a silent allow/deny guess); this middleware contributes
// the HTTP-shaped ritual around it — host resolution with its
// 401/400/404 answers — and maps the returned sentinels onto the error
// envelope the same way the web envelope unwraps AppErrors.
//
// Semantics mirror RequireHostAccess so the two compose without
// surprises:
//
//   - Unknown host ID → 404 HOST_NOT_FOUND (checked here again,
//     so the gate is correct even if mounted outside the subtree).
//   - Stored level below `required` — including no row at all — →
//     403 ACCESS_DENIED (permission.RequireLevel).
//   - User-ID-less request → 401, RequireAdmin's contract for a
//     miswired route.
//   - A failing lookup gets 500, never a silent allow/deny guess
//     (permission.RequireLevel).
func RequireHostPermission(
	hostRepo hostRepoGetter,
	accessRepo permission.PermissionGetter,
	required string,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hostID, userID, ok := resolveHostAndCaller(w, r, hostRepo)
			if !ok {
				return
			}

			if err := permission.RequireLevel(accessRepo, hostID, userID, required); err != nil {
				appErr := apperrors.ErrInternalError
				if mapped, ok := errors.AsType[*apperrors.AppError](err); ok {
					appErr = mapped
				}
				apperrors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
