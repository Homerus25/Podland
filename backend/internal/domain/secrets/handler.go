package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// SecretHandler handles secret requests. All endpoints are JSON and
// cross the endpoint envelope (internal/web).
type SecretHandler struct {
	service *SecretService
}

// NewSecretHandler creates a new SecretHandler
func NewSecretHandler(service *SecretService) *SecretHandler {
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
// statement on this path: a decode failure returns without logging (json
// syntax errors quote the offending character, and any fragment of the
// body may be a fragment of the value), validator errors name the failing
// field, not its content, and the response echoes only the created ID.
func (h *SecretHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (*SecretCreateResponse, error) {
		var request SecretCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			return nil, errors.ErrInvalidRequest.WithMessage("Invalid request body")
		}

		request.Name = strings.TrimSpace(request.Name)

		validate := validator.New()
		if err := validate.Struct(request); err != nil {
			return nil, errors.NewAppError("VALIDATION_FAILED", err.Error(), http.StatusBadRequest)
		}
		if !secretNamePattern.MatchString(request.Name) {
			return nil, errors.NewAppError("VALIDATION_FAILED",
				"Name must start with a letter or digit and contain only letters, digits, underscores, dots, or dashes",
				http.StatusBadRequest)
		}
		if len(request.Value) > maxSecretValueBytes {
			return nil, errors.NewAppError("VALIDATION_FAILED",
				fmt.Sprintf("Value must be at most %d bytes", maxSecretValueBytes),
				http.StatusBadRequest)
		}

		return h.service.Create(ctx, userID, machineID, request)
	})
}

// Delete handles removing a secret by name or ID. Route-level
// RequireMachinePermission(write) (app.go) gates it at write; on success
// podman answers 204 and this handler echoes that with an empty body —
// the frontend series (170) invalidates the list on the 204.
func (h *SecretHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, machineID string) (struct{}, error) {
		secretID := chi.URLParam(r, "secretId")
		if secretID == "" {
			return struct{}{}, errors.NewAppError("MISSING_SECRET_ID", "Secret ID is required", http.StatusBadRequest)
		}

		return struct{}{}, h.service.Delete(ctx, userID, machineID, secretID)
	})
}

// List handles listing secret metadata
func (h *SecretHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
		return h.service.List(ctx, userID, machineID)
	})
}

// Get handles inspecting a single secret by name or ID. ?reveal=true asks
// for the secret's value; the admin-only gate lives in the service (the
// reveal is a query param, so route-level middleware cannot see it), which
// answers 403 ACCESS_DENIED for callers below admin. The response body is
// never logged: a reveal's JSON carries the value.
func (h *SecretHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*SecretInspectResponse, error) {
		secretID := chi.URLParam(r, "secretId")
		if secretID == "" {
			return nil, errors.NewAppError("MISSING_SECRET_ID", "Secret ID is required", http.StatusBadRequest)
		}

		reveal := r.URL.Query().Get("reveal") == "true"
		return h.service.Get(ctx, userID, machineID, secretID, reveal)
	})
}
