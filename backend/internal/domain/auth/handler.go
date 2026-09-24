package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/web"
)

// AuthHandler handles authentication requests
type AuthHandler struct {
	service  *AuthService
	userRepo models.UserRepositoryInterface
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(service *AuthService, userRepo models.UserRepositoryInterface) *AuthHandler {
	return &AuthHandler{
		service:  service,
		userRepo: userRepo,
	}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse represents a login response. Role mirrors
// StatusResponse's: read from the user record, so the client can gate
// admin surfaces without a second round trip.
type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// refreshTokenCookieName is the httpOnly cookie login sets to carry the
// opaque refresh token. It is scoped to /api/auth so the browser sends
// it only to the auth endpoints that consume it.
const refreshTokenCookieName = "refresh_token"

// newRefreshCookie builds the refresh cookie carrying the spec's four
// attributes — httpOnly (out of JavaScript reach), Secure,
// SameSite=Strict, and Path=/api/auth so the browser sends it only to
// auth endpoints. The cookie login and refresh hand out and the one
// logout clears with are built here, so their attributes can never
// drift apart.
func newRefreshCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    value,
		Path:     "/api/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}
}

// setRefreshCookie writes the refresh token as the httpOnly cookie
// login and refresh hand to the client.
func setRefreshCookie(w http.ResponseWriter, refreshToken string) {
	http.SetCookie(w, newRefreshCookie(refreshToken))
}

// Login handles user login: it shapes the request and response, while
// credential verification, token issuance, and session persistence live
// in AuthService.Login. The auth surface has no authenticated caller, so
// the closure's user ID is unused; the refresh cookie is set inside the
// closure (headers only) — the envelope owns the status line and body.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (LoginResponse, error) {
		var req LoginRequest
		if appErr := web.Decode(r, &req); appErr != nil {
			return LoginResponse{}, appErr
		}

		token, refreshToken, user, err := h.service.Login(req.Username, req.Password)
		if err != nil {
			return LoginResponse{}, err
		}

		// The refresh token exists only inside this cookie: httpOnly keeps
		// it out of JavaScript reach, the narrow path keeps it out of every
		// non-auth request, and SameSite=Strict + Secure harden it against
		// cross-site sending and plaintext transport.
		setRefreshCookie(w, refreshToken)

		return LoginResponse{Token: token, Username: user.Username, Role: user.Role}, nil
	})
}

// RefreshResponse represents a refresh response. It intentionally
// matches the login body: the new access JWT and the username and role
// it belongs to — the refresh token lives only in the rotated cookie.
type RefreshResponse = LoginResponse

// Refresh handles the unauthenticated renewal path: it exchanges the
// httpOnly refresh cookie for a new access JWT in the body and a
// rotated refresh cookie. A missing cookie, an unknown token (replayed
// or revoked), and an expired session are all 401s in the structured
// error envelope — the browser-facing behavior is a silent re-login
// prompt, not an error screen.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (RefreshResponse, error) {
		cookie, err := r.Cookie(refreshTokenCookieName)
		if err != nil {
			return RefreshResponse{}, errors.ErrRefreshInvalid
		}

		token, refreshToken, user, err := h.service.Refresh(cookie.Value)
		if err != nil {
			return RefreshResponse{}, err
		}

		setRefreshCookie(w, refreshToken)

		return RefreshResponse{Token: token, Username: user.Username, Role: user.Role}, nil
	})
}

// clearRefreshCookie expires the refresh cookie: the same name, path,
// and attributes login and refresh set, but empty and already in the
// past, so the browser deletes it whether or not a session stood
// behind it.
func clearRefreshCookie(w http.ResponseWriter) {
	cookie := newRefreshCookie("")
	cookie.Expires = time.Unix(0, 0)
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
}

// Logout handles user logout. When the request carries a refresh
// cookie, the session behind it is revoked server-side; the cookie is
// cleared in every case — before revocation runs, inside the closure —
// so even a storage failure cannot leave the browser holding a token
// its user tried to give up. Logout is idempotent: no cookie, or a
// cookie whose session is already gone, still succeeds with the same
// response — the end state (no session, no cookie) is what the caller
// asked for.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (map[string]string, error) {
		clearRefreshCookie(w)

		if cookie, err := r.Cookie(refreshTokenCookieName); err == nil {
			if err := h.service.Logout(cookie.Value); err != nil {
				return nil, err
			}
		}

		return map[string]string{"message": "Logged out successfully"}, nil
	})
}

// ChangePasswordRequest represents a self-service password change: the
// current password proves the caller's authority, the new one replaces
// the hash. Policy (length) and verification live in the service.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" validate:"required"`
	NewPassword     string `json:"newPassword" validate:"required"`
}

// ChangePasswordResponse represents a password change response. It
// matches the login body: the caller's fresh access JWT and username
// and role — the fresh session travels only in the rotated cookie.
type ChangePasswordResponse = LoginResponse

// ChangePassword handles POST /api/auth/password: the authenticated
// caller changes their own password. The service verifies the current
// password, enforces the length policy, revokes every session of the
// user, and issues the fresh login this handler hands back (body +
// refresh cookie) so the changing device stays signed in. Route
// registration sits behind JWTMiddleware in internal/app — the envelope
// passes the caller's user ID through.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (ChangePasswordResponse, error) {
		var req ChangePasswordRequest
		if appErr := web.Decode(r, &req); appErr != nil {
			return ChangePasswordResponse{}, appErr
		}

		token, refreshToken, user, err := h.service.ChangePassword(userID, req.CurrentPassword, req.NewPassword)
		if err != nil {
			return ChangePasswordResponse{}, err
		}

		setRefreshCookie(w, refreshToken)

		return ChangePasswordResponse{Token: token, Username: user.Username, Role: user.Role}, nil
	})
}

// StatusResponse represents authentication status response
//
// Role is the authenticated user's role ("user" or "admin"), read
// from the database on each call — not a JWT claim — so role changes
// take effect immediately rather than at token expiry. It is omitted
// when the user record can no longer be found (e.g. deleted while a
// token was still valid).
//
// NeedsSetup reports a fresh instance: true only for an unauthenticated
// caller on an instance whose users table is empty — evaluated per
// request, so it flips to false the moment setup completes. The
// frontend's boot probe steers routing on it (login → setup while
// setup is needed); authenticated callers always get false, because a
// configured instance is the only state they can exist in.
type StatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	NeedsSetup    bool   `json:"needsSetup"`
	UserID        string `json:"userId,omitempty"`
	Role          string `json:"role,omitempty"`
}

// Status handles authentication status check. The caller's user ID
// passes through as-is: empty means an anonymous caller (the route is
// served without a token too) and answers {authenticated: false} plus
// whether the instance still needs first-run setup.
func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (StatusResponse, error) {
		if userID == "" {
			count, err := h.userRepo.Count()
			if err != nil {
				return StatusResponse{}, err
			}
			return StatusResponse{Authenticated: false, NeedsSetup: count == 0}, nil
		}

		user, err := h.userRepo.GetByID(userID)
		if err != nil {
			return StatusResponse{}, err
		}

		resp := StatusResponse{
			Authenticated: true,
			UserID:        userID,
		}
		if user != nil {
			resp.Role = user.Role
		}
		return resp, nil
	})
}

// SetupRequest represents a first-run setup request: the credentials of
// the instance's first user. Validation mirrors login (required
// fields); the password policy lives in the creation service.
type SetupRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// SetupResponse represents a setup response. It matches the login
// body — setup signs the caller in on the spot — and the refresh
// session travels only in the cookie, as at login.
type SetupResponse = LoginResponse

// Setup handles POST /api/setup/user, the unauthenticated first-run
// bootstrap: on a fresh instance (zero users) it creates the first user
// with the admin role and responds with the login payload plus the
// refresh cookie, so the setup page adopts the session without a second
// round trip. The first-user guard lives in the creation service and is
// atomic: exactly one concurrent caller wins; every other caller — and
// any call on an already-configured instance — receives 409
// SETUP_COMPLETED with nothing written. Route registration sits in the
// per-IP auth rate-limit group in internal/app; the closure's user ID
// is unused (no caller exists during setup).
func (h *AuthHandler) Setup(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (SetupResponse, error) {
		var req SetupRequest
		if appErr := web.Decode(r, &req); appErr != nil {
			return SetupResponse{}, appErr
		}

		token, refreshToken, user, err := h.service.Setup(req.Username, req.Password)
		if err != nil {
			return SetupResponse{}, err
		}

		setRefreshCookie(w, refreshToken)

		return SetupResponse{Token: token, Username: user.Username, Role: user.Role}, nil
	})
}
