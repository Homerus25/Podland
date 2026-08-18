package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/yourorg/podland/backend/internal/contextutils"
	apperrors "github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/models"
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
			requestID := contextutils.GetRequestID(r.Context())
			userID := contextutils.GetUserIDFromContext(r.Context())
			if userID == "" {
				apperrors.RespondWithError(w, apperrors.ErrTokenInvalid, requestID)
				return
			}

			machineID := chi.URLParam(r, "id")
			if machineID == "" {
				apperrors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", requestID)
				return
			}

			machine, err := machineRepo.GetByID(machineID)
			if err != nil {
				apperrors.RespondWithError(w, apperrors.ErrInternalError, requestID)
				return
			}
			if machine == nil {
				apperrors.RespondWithError(w, apperrors.ErrMachineNotFound, requestID)
				return
			}

			hasAccess, err := accessRepo.HasAccess(machineID, userID)
			if err != nil {
				apperrors.RespondWithError(w, apperrors.ErrInternalError, requestID)
				return
			}
			if !hasAccess {
				apperrors.RespondWithError(w, apperrors.ErrAccessDenied, requestID)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
