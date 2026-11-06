package secrets

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/Homerus25/podland/backend/internal/bulkremove"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/web"
	"go.podman.io/podman/v6/pkg/domain/entities"
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

// Create handles creating a secret. Decode validates the untrimmed name;
// the handler then trims and applies its pattern and byte-cap checks (so a
// whitespace-only name still answers VALIDATION_FAILED via the pattern
// check, while a name pushed over max=255 only by its padding now fails
// Decode's max tag — the more defensible edge). The value is validated
// as-is and forwarded byte-identical. Neither the request DTO nor the
// value ever reaches a log statement on this path: a decode failure
// returns without logging (json syntax errors quote the offending
// character, and any fragment of the body may be a fragment of the
// value), validator errors name the failing field, not its content, and
// the response echoes only the created ID.
func (h *SecretHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, hostID string) (*SecretCreateResponse, error) {
		var request SecretCreateRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return nil, appErr
		}

		request.Name = strings.TrimSpace(request.Name)

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

		return h.service.Create(ctx, userID, hostID, request)
	})
}

// Delete handles removing a secret by name or ID. Route-level
// RequireHostPermission(write) (app.go) gates it at write; on success
// podman answers 204 and this handler echoes that with an empty body —
// the frontend series (170) invalidates the list on the 204.
func (h *SecretHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, hostID string) (struct{}, error) {
		secretID, guard := web.URLParam(r, "secretId", "MISSING_SECRET_ID")
		if guard != nil {
			return struct{}{}, guard
		}

		return struct{}{}, h.service.Delete(ctx, userID, hostID, secretID)
	})
}

// BulkRemove handles removing several secrets in one request. Route-level
// RequireHostPermission(write) (app.go) gates it at write. The body
// carries the secret names (the `ids` field — the same identity the
// single-item Delete's {secretId} route addresses) and nothing else —
// this endpoint's contract has
// no force parameter at all (bulkremove.IDsRequest), because the
// single-item Delete has none either. The answer is 200 with one report
// entry per requested id, in request order — a per-id failure is an
// entry, never a status code. Every entry rides the single-item Delete's
// classification (internal/removal through internal/bulkremove), so an
// in-use secret answers a SECRET_IN_USE entry carrying podman's message
// while the rest of the batch still processes, and there is no force
// retry that could remove it.
func (h *SecretHandler) BulkRemove(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) ([]bulkremove.Entry, error) {
		var request bulkremove.IDsRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return nil, appErr
		}

		return bulkremove.Run(ctx, request.IDs, func(ctx context.Context, secretID string) error {
			return h.service.Delete(ctx, userID, hostID, secretID)
		}), nil
	})
}

// List handles listing secret metadata
func (h *SecretHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) ([]SecretListResponse, error) {
		return h.service.List(ctx, userID, hostID)
	})
}

// Get handles inspecting a single secret by name or ID. ?reveal=true asks
// for the secret's value; the admin-only gate lives in the service (the
// reveal is a query param, so route-level middleware cannot see it), which
// answers 403 ACCESS_DENIED for callers below admin. The response body is
// never logged: a reveal's JSON carries the value.
func (h *SecretHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*SecretInspectResponse, error) {
		secretID, guard := web.URLParam(r, "secretId", "MISSING_SECRET_ID")
		if guard != nil {
			return nil, guard
		}

		reveal := r.URL.Query().Get("reveal") == "true"
		return h.service.Get(ctx, userID, hostID, secretID, reveal)
	})
}

// Inspect handles the raw inspect window (issue 287): 200 with the
// secret's raw podman inspect report as the top-level JSON body — metadata
// only. The service never passes reveal, so the response cannot carry the
// value; the route is read-level (no reveal/admin gate, unlike Get).
func (h *SecretHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*entities.SecretInfoReport, error) {
		secretID, guard := web.URLParam(r, "secretId", "MISSING_SECRET_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.Inspect(ctx, userID, hostID, secretID)
	})
}

// MovePreview answers the copy/move dialog's facts (issue 239,
// doku/description_secrets_api.md): collision, referencing quadlets.
// Route-level RequireHostAccess (app.go) gates the subtree at read;
// the target host's read access is enforced inside the service
// against the access store.
func (h *SecretHandler) MovePreview(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*SecretMovePreviewResponse, error) {
		secretID, guard := web.URLParam(r, "secretId", "MISSING_SECRET_ID")
		if guard != nil {
			return nil, guard
		}

		targetHostID, guard := web.QueryString(r, "targetHostId", "MISSING_TARGET_HOST_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.MovePreview(ctx, userID, hostID, secretID, targetHostID)
	})
}

// Move handles the cross-host copy/move transfer of a secret (issue
// 239, doku/description_secrets_api.md). The route sits at the subtree's
// write gate for the SOURCE host; the value leaving it is a reveal,
// so the service additionally gates the source at admin and the target
// host's write-level access in-service against the access store.
//
// Transport: the plain JSON envelope — secret values are small, the
// NDJSON transport stays a volume concern. Neither the request DTO nor
// the response ever reaches a log statement on this path (the value
// travels in the create the transfer issues, never in a response body).
func (h *SecretHandler) Move(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*SecretMoveResponse, error) {
		secretID, guard := web.URLParam(r, "secretId", "MISSING_SECRET_ID")
		if guard != nil {
			return nil, guard
		}

		var request SecretMoveRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.Move(ctx, userID, hostID, secretID, request)
	})
}
