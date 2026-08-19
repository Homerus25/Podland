package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/models"
)

// AuthHandler handles authentication requests
type AuthHandler struct {
	service      *AuthService
	userRepo     models.UserRepositoryInterface
	errorHandler *errors.ErrorHandler
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(service *AuthService, userRepo models.UserRepositoryInterface) *AuthHandler {
	return &AuthHandler{
		service:      service,
		userRepo:     userRepo,
		errorHandler: errors.NewErrorHandler(),
	}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token string `json:"token"`
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
// in AuthService.Login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	requestID := contextutils.GetRequestID(r.Context())

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.RespondWithError(w, errors.ErrInvalidRequest, requestID)
		return
	}

	// Validate required fields
	if req.Username == "" || req.Password == "" {
		errors.RespondWithError(w, errors.ErrMissingRequiredFields, requestID)
		return
	}

	token, refreshToken, _, err := h.service.Login(req.Username, req.Password)
	if err != nil {
		errors.RespondWithError(w, h.errorHandler.HandleError(err), requestID)
		return
	}

	// The refresh token exists only inside this cookie: httpOnly keeps
	// it out of JavaScript reach, the narrow path keeps it out of every
	// non-auth request, and SameSite=Strict + Secure harden it against
	// cross-site sending and plaintext transport.
	setRefreshCookie(w, refreshToken)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LoginResponse{Token: token})
}

// RefreshResponse represents a refresh response. It intentionally
// matches the login body: exactly the new access JWT — the refresh
// token lives only in the rotated cookie.
type RefreshResponse = LoginResponse

// Refresh handles the unauthenticated renewal path: it exchanges the
// httpOnly refresh cookie for a new access JWT in the body and a
// rotated refresh cookie. A missing cookie, an unknown token (replayed
// or revoked), and an expired session are all 401s in the structured
// error envelope — the browser-facing behavior is a silent re-login
// prompt, not an error screen.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	requestID := contextutils.GetRequestID(r.Context())

	cookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		errors.RespondWithError(w, errors.ErrRefreshInvalid, requestID)
		return
	}

	token, refreshToken, err := h.service.Refresh(cookie.Value)
	if err != nil {
		errors.RespondWithError(w, h.errorHandler.HandleError(err), requestID)
		return
	}

	setRefreshCookie(w, refreshToken)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(RefreshResponse{Token: token})
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
// cleared in every case — before revocation runs — so even a storage
// failure cannot leave the browser holding a token its user tried to
// give up. Logout is idempotent: no cookie, or a cookie whose session
// is already gone, still succeeds with the same response — the end
// state (no session, no cookie) is what the caller asked for.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	requestID := contextutils.GetRequestID(r.Context())

	clearRefreshCookie(w)

	if cookie, err := r.Cookie(refreshTokenCookieName); err == nil {
		if err := h.service.Logout(cookie.Value); err != nil {
			errors.RespondWithError(w, h.errorHandler.HandleError(err), requestID)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out successfully"})
}

// StatusResponse represents authentication status response
//
// Role is the authenticated user's role ("user" or "admin"), read
// from the database on each call — not a JWT claim — so role changes
// take effect immediately rather than at token expiry. It is omitted
// when the user record can no longer be found (e.g. deleted while a
// token was still valid).
type StatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	UserID        string `json:"userId,omitempty"`
	Role          string `json:"role,omitempty"`
}

// Status handles authentication status check
func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	// Check if user is authenticated by looking for user ID in context
	userID := contextutils.GetUserIDFromContext(r.Context())

	if userID != "" {
		user, err := h.userRepo.GetByID(userID)
		if err != nil {
			errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
			return
		}

		resp := StatusResponse{
			Authenticated: true,
			UserID:        userID,
		}
		if user != nil {
			resp.Role = user.Role
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(StatusResponse{
			Authenticated: false,
		})
	}
}
