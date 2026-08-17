package auth

import (
	"encoding/json"
	"net/http"

	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
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
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token string `json:"token"`
}

// Login handles user login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.RespondWithError(w, errors.ErrInvalidRequest, contextutils.GetRequestID(r.Context()))
		return
	}

	// Validate required fields
	if req.Username == "" || req.Password == "" {
		errors.RespondWithError(w, errors.ErrMissingRequiredFields, contextutils.GetRequestID(r.Context()))
		return
	}

	// Get user from database
	user, err := h.userRepo.GetByUsername(req.Username)
	if err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}
	if user == nil {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// Verify password hash
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// Generate JWT token
	token, err := h.service.GenerateToken(user.ID)
	if err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Return token
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LoginResponse{Token: token})
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// For JWT, logout is typically handled client-side by removing the token
	// Server-side, we could implement token blacklisting if needed
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
