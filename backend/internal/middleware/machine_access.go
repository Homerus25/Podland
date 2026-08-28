package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/Homerus25/podland/backend/internal/contextutils"
	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
)

// machineRepoGetter is the slice of the machine repository
// RequireMachineAccess needs: resolving a machine ID. Declared locally
// (rather than importing the domain interface) so the middleware
// depends only on the two lookups it actually performs; the concrete
// *models.MachineRepository and the test mocks satisfy it structurally.
type machineRepoGetter interface {
	GetByID(id string) (*models.Machine, error)
}

// accessRepoChecker is the slice of the machine-access repository
// RequireMachineAccess needs: the boolean access probe shared with
// MachineService.UserHasAccess.
type accessRepoChecker interface {
	HasAccess(machineID, userID string) (bool, error)
}

// accessRepoPermissionGetter is the slice of the machine-access
// repository RequireMachinePermission needs: the caller's stored
// permission level (empty string when no row exists).
type accessRepoPermissionGetter interface {
	GetPermission(machineID, userID string) (string, error)
}

// resolveMachineAndCaller performs the checks RequireMachineAccess and
// RequireMachinePermission share: the request must carry an
// authenticated user ID (401 otherwise, RequireAdmin's contract for a
// miswired route), a machine ID URL param (400), and a known machine
// (404 MACHINE_NOT_FOUND, the machines domain's answer, uniformly
// across the subtree). On any failure it has already written the error
// response and returns ok=false; on success it returns the machine and
// user IDs the access probe needs.
func resolveMachineAndCaller(w http.ResponseWriter, r *http.Request, machineRepo machineRepoGetter) (machineID, userID string, ok bool) {
	requestID := contextutils.GetRequestID(r.Context())
	userID = contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		apperrors.RespondWithError(w, apperrors.ErrTokenInvalid, requestID)
		return "", "", false
	}

	machineID = chi.URLParam(r, "id")
	if machineID == "" {
		apperrors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", requestID)
		return "", "", false
	}

	machine, err := machineRepo.GetByID(machineID)
	if err != nil {
		apperrors.RespondWithError(w, apperrors.ErrInternalError, requestID)
		return "", "", false
	}
	if machine == nil {
		apperrors.RespondWithError(w, apperrors.ErrMachineNotFound, requestID)
		return "", "", false
	}

	return machineID, userID, true
}

// RequireMachineAccess gates a route behind the caller's machine_access
// row. It must run after JWTMiddleware (it reads the authenticated user
// ID from the request context) and is designed for the
// /api/machines/{id} subtree, taking the machine ID from the chi URL
// parameter — so one seam covers the machines endpoints and every
// downstream family that rides on a machine (containers, images,
// quadlets, events) including ones added later.
//
// Semantics, mirroring the machines domain (MachineService.
// GetMachineByID / Delete):
//
//   - Unknown machine ID → 404 MACHINE_NOT_FOUND, the same answer the
//     machines endpoints give, uniformly across the subtree (the
//     downstream services used to surface this as 500 or 502).
//   - Known machine, no access row → 403 ACCESS_DENIED. Access is
//     resolved per request from the database — like RequireAdmin's role
//     lookup — so grants and revocations take effect immediately
//     rather than at token expiry.
//   - A caller that is authenticated but user-ID-less surfaces as 401,
//     matching RequireAdmin: an unauthenticated caller, not an
//     authorization decision.
//   - A failing lookup gets 500 rather than a silent allow/deny guess.
//
// The machines handlers keep their own service-level checks; this
// middleware is the outer gate that extends the same contract to the
// downstream endpoints, which previously trusted the JWT alone.
func RequireMachineAccess(
	machineRepo machineRepoGetter,
	accessRepo accessRepoChecker,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			machineID, userID, ok := resolveMachineAndCaller(w, r, machineRepo)
			if !ok {
				return
			}

			hasAccess, err := accessRepo.HasAccess(machineID, userID)
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

// RequireMachinePermission gates a route behind the LEVEL of the
// caller's machine_access row rather than its mere existence. Use it
// inside the RequireMachineAccess-protected /api/machines/{id} subtree
// on mutating routes: RequireMachineAccess stays the subtree-level
// read gate, this adds "the stored permission must be at least
// `required`" (read < write < admin, via models.HasAtLeast) for
// endpoints that change remote state — container create/delete/start/
// stop, image pull/delete, quadlet create/import/update/delete/start/
// stop/restart, and later secret writes.
//
// Semantics mirror RequireMachineAccess so the two compose without
// surprises:
//
//   - Unknown machine ID → 404 MACHINE_NOT_FOUND (checked here again,
//     so the gate is correct even if mounted outside the subtree).
//   - Stored level below `required` — including no row at all — →
//     403 ACCESS_DENIED.
//   - User-ID-less request → 401, RequireAdmin's contract for a
//     miswired route.
//   - A failing lookup gets 500, never a silent allow/deny guess.
func RequireMachinePermission(
	machineRepo machineRepoGetter,
	accessRepo accessRepoPermissionGetter,
	required string,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			machineID, userID, ok := resolveMachineAndCaller(w, r, machineRepo)
			if !ok {
				return
			}

			actual, err := accessRepo.GetPermission(machineID, userID)
			if err != nil {
				apperrors.RespondWithError(w, apperrors.ErrInternalError, contextutils.GetRequestID(r.Context()))
				return
			}
			if !models.HasAtLeast(required, actual) {
				apperrors.RespondWithError(w, apperrors.ErrAccessDenied, contextutils.GetRequestID(r.Context()))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
