package middleware

import (
	"net/http"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
)

// RequireAdmin gates a route behind the admin role. It must run after
// JWTMiddleware (it reads the authenticated user ID from the request
// context) and resolves the role from the database on every request —
// deliberately not a JWT claim — so role changes take effect
// immediately rather than at token expiry.
//
// A caller that is authenticated but not an admin (including one whose
// user record no longer exists) gets 403. A request that never passed
// JWTMiddleware surfaces as 401 — that is an unauthenticated caller,
// and answering 403 would mask the auth contract on a miswired route.
// A failing lookup gets 500 rather than a silent allow/deny guess.
func RequireAdmin(userRepo models.UserRepositoryInterface) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := contextutils.GetRequestID(r.Context())
			userID := contextutils.GetUserIDFromContext(r.Context())
			if userID == "" {
				apperrors.RespondWithError(w, apperrors.ErrTokenInvalid, requestID)
				return
			}

			user, err := userRepo.GetByID(userID)
			if err != nil {
				apperrors.RespondWithError(w, apperrors.ErrInternalError, requestID)
				return
			}
			if user == nil || user.Role != models.RoleAdmin {
				apperrors.RespondWithError(w, apperrors.ErrAdminRequired, requestID)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
