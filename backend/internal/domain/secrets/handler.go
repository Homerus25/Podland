package secrets

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
)

// SecretHandler handles secret requests
type SecretHandler struct {
	service SecretServiceInterface
}

// NewSecretHandler creates a new SecretHandler
func NewSecretHandler(service SecretServiceInterface) *SecretHandler {
	return &SecretHandler{
		service: service,
	}
}

// List handles listing secret metadata
func (h *SecretHandler) List(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "List secrets")

	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), errors.ErrInvalidCredentials.Code)
		return
	}

	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	secrets, err := h.service.List(r.Context(), userID, machineID)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "list", slog.Any("error", err), slog.String("userID", userID), slog.String("machineID", machineID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(secrets)
}

// Get handles inspecting a single secret by name or ID. ?reveal=true asks
// for the secret's value; the admin-only gate lives in the service (the
// reveal is a query param, so route-level middleware cannot see it), which
// answers 403 ACCESS_DENIED for callers below admin. The response body is
// never logged: a reveal's JSON carries the value.
func (h *SecretHandler) Get(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Inspect secret")

	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), errors.ErrInvalidCredentials.Code)
		return
	}

	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	secretID := chi.URLParam(r, "secretId")
	if secretID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_SECRET_ID", "Secret ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	reveal := r.URL.Query().Get("reveal") == "true"

	secret, err := h.service.Get(r.Context(), userID, machineID, secretID, reveal)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "get", slog.Any("error", err), slog.String("userID", userID), slog.String("machineID", machineID), slog.String("secretId", secretID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(secret)
}
