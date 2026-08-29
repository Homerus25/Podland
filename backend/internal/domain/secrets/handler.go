package secrets

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
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

// secretNamePattern is podman's secret name rule: a letter or digit first,
// then letters, digits, underscores, dots, dashes. Enforced as an explicit
// check next to validate.Struct because go-playground/validator has no
// regex tag; kept in lockstep with the frontend's SECRET_NAME_PATTERN.
var secretNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// maxSecretValueBytes is the create payload cap (1 MiB). Enforced as an
// explicit len() check because validator's min/max count runes for
// strings, and the cap — like podman's own size limit — is in bytes.
const maxSecretValueBytes = 1048576

// Create handles creating a secret. The name is trimmed BEFORE validation
// so the rules apply to the trimmed form (a padded-but-valid name must not
// be rejected for its padding); the value is validated as-is and forwarded
// byte-identical. Neither the request DTO nor the value ever reaches a log
// statement on this path: validator errors name the failing field, not its
// content, and the response echoes only the created ID.
func (h *SecretHandler) Create(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Create secret")

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

	var request SecretCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", contextutils.GetRequestID(r.Context()))
		// The decode error is deliberately NOT logged: json syntax errors
		// quote the offending character from the request body, and any
		// fragment of the body may be a fragment of the value.
		slog.ErrorContext(r.Context(), "Invalid secret create request body")
		return
	}

	request.Name = strings.TrimSpace(request.Name)

	validate := validator.New()
	if err := validate.Struct(request); err != nil {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), contextutils.GetRequestID(r.Context()))
		// Safe to log: validator errors name the failing field and rule,
		// never the field's content.
		slog.ErrorContext(r.Context(), "Validation failed", slog.Any("error", err))
		return
	}
	if !secretNamePattern.MatchString(request.Name) {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "VALIDATION_FAILED",
			"Name must start with a letter or digit and contain only letters, digits, underscores, dots, or dashes",
			contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "Validation failed", slog.String("field", "name"), slog.String("rule", "pattern"))
		return
	}
	if len(request.Value) > maxSecretValueBytes {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "VALIDATION_FAILED",
			fmt.Sprintf("Value must be at most %d bytes", maxSecretValueBytes),
			contextutils.GetRequestID(r.Context()))
		// Only the size class is logged — never the payload it describes.
		slog.ErrorContext(r.Context(), "Validation failed",
			slog.String("field", "value"), slog.Int("bytes", len(request.Value)))
		return
	}

	secret, err := h.service.Create(r.Context(), userID, machineID, request)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "create", slog.Any("error", err), slog.String("userID", userID), slog.String("machineID", machineID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(secret)
}

// Delete handles removing a secret by name or ID. Route-level
// RequireMachinePermission(write) (app.go) gates it at write; on success
// podman answers 204 and this handler echoes that with an empty body —
// the frontend series (170) invalidates the list on the 204.
func (h *SecretHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Delete secret")

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

	err := h.service.Delete(r.Context(), userID, machineID, secretID)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "delete", slog.Any("error", err), slog.String("userID", userID), slog.String("machineID", machineID), slog.String("secretId", secretID))
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
