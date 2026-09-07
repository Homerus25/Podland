package secrets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Homerus25/podland/backend/internal/connections"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	podmanSecrets "go.podman.io/podman/v6/pkg/bindings/secrets"
)

// Compile-time check that *SecretService satisfies SecretServiceInterface.
var _ SecretServiceInterface = (*SecretService)(nil)

// SecretServiceInterface defines the interface for secret services.
// This interface is used for dependency injection and testing.
type SecretServiceInterface interface {
	List(ctx context.Context, userID, machineID string) ([]SecretListResponse, error)
	Get(ctx context.Context, userID, machineID, secretID string, reveal bool) (*SecretInspectResponse, error)
	Create(ctx context.Context, userID, machineID string, req SecretCreateRequest) (*SecretCreateResponse, error)
	Delete(ctx context.Context, userID, machineID, secretID string) error
}

// accessPermissionGetter is the slice of the machine-access repository
// SecretService needs: the caller's stored permission level (empty string
// when no row exists). Declared locally — mirroring the middleware's
// accessRepoPermissionGetter — so the service depends only on the lookup
// it performs; *models.MachineAccessRepository and the test mocks satisfy
// it structurally.
type accessPermissionGetter interface {
	GetPermission(machineID, userID string) (string, error)
}

// SecretService provides business logic for secrets. Secrets live only in
// the remote machine's podman; this service proxies podman directly and
// persists nothing locally, matching ContainerService/ImageService/
// QuadletService.
type SecretService struct {
	conn connections.Manager
	// accessRepo resolves the caller's permission level for the
	// admin-only value reveal (?reveal=true): the gate is a query param,
	// not a separate route, so it cannot ride on route-level middleware.
	accessRepo accessPermissionGetter
}

// NewSecretService creates a new SecretService with the given connection
// manager interface and machine-access repository (permission lookup for
// the admin-only reveal).
func NewSecretService(conn connections.Manager, accessRepo accessPermissionGetter) *SecretService {
	return &SecretService{
		conn:       conn,
		accessRepo: accessRepo,
	}
}

// List returns the metadata of the secrets on the specified machine.
func (s *SecretService) List(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
	var response []SecretListResponse
	err := s.conn.WithConnection(ctx, userID, machineID, "secrets.List", func(connCtx context.Context) error {
		reports, err := podmanSecrets.List(connCtx, nil)
		if err != nil {
			return fmt.Errorf("failed to list secrets: %w", err)
		}

		response = ToSecretListResponse(reports)
		return nil
	})
	return response, err
}

// Get returns a single secret on the specified machine. With reveal=false
// (or for callers below admin) it is metadata-only: podman's Inspect runs
// without WithShowSecret and the mapped response cannot carry a value.
// With reveal=true for an admin caller it asks podman for the value
// (WithShowSecret), maps it into SecretData, and records a value-free
// reveal event.
func (s *SecretService) Get(ctx context.Context, userID, machineID, secretID string, reveal bool) (*SecretInspectResponse, error) {
	// The reveal gate is admin-only and fires BEFORE the connection lease:
	// a below-admin caller is refused outright (403 ACCESS_DENIED, not a
	// silent metadata degrade) and podman is never asked for the value.
	// The subtree-level RequireMachineAccess already proved some access
	// row exists; GetPermission re-reads the LEVEL so revocations and
	// upgrades take effect immediately. A failing lookup is a 500, never
	// a silent allow/deny guess (the middleware's contract).
	if reveal {
		level, err := s.accessRepo.GetPermission(machineID, userID)
		if err != nil {
			return nil, appErrors.ErrInternalError
		}
		if !models.HasAtLeast(models.PermissionAdmin, level) {
			return nil, appErrors.ErrAccessDenied
		}
	}

	var response *SecretInspectResponse
	err := s.conn.WithConnection(ctx, userID, machineID, "secrets.Get", func(connCtx context.Context) error {
		opts := new(podmanSecrets.InspectOptions)
		if reveal {
			opts = opts.WithShowSecret(true)
		}

		report, err := podmanSecrets.Inspect(connCtx, secretID, opts)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrSecretNotFound
			}
			return fmt.Errorf("failed to inspect secret: %w", err)
		}

		if reveal {
			// Audit event per the redaction discipline: who revealed what,
			// never the value itself.
			slog.InfoContext(ctx, "secret value revealed",
				slog.String("userID", userID),
				slog.String("machineID", machineID),
				slog.String("secretID", secretID))
		}

		response = ToSecretInspectResponse(report, reveal)
		return nil
	})
	return response, err
}

// Create installs a new secret on the specified machine. Podman takes the
// secret's value as the raw request body reader and the name in options
// (secrets.Create), so the value is handed over via strings.NewReader —
// byte-identical, never trimmed — under the file driver with no labels
// (v1 ruling: driver/labels deferred). The response carries only the
// created ID. A duplicate name surfaces as ErrSecretAlreadyExists (podman
// reports the conflict as a 500 ErrorModel whose cause carries the common
// package's "secret name in use" sentinel, classified by the adapter's
// cause check); any other creation failure becomes
// ErrSecretCreationFailed. The value itself never appears in any
// log statement on this path.
func (s *SecretService) Create(ctx context.Context, userID, machineID string, req SecretCreateRequest) (*SecretCreateResponse, error) {
	var response *SecretCreateResponse
	err := s.conn.WithConnection(ctx, userID, machineID, "secrets.Create", func(connCtx context.Context) error {
		opts := new(podmanSecrets.CreateOptions).WithName(req.Name)

		report, err := podmanSecrets.Create(connCtx, strings.NewReader(req.Value), opts)
		if err != nil {
			if podmanerr.IsAlreadyExists(err) {
				return appErrors.ErrSecretAlreadyExists
			}
			// The underlying error is logged here, not returned: the sentinel
			// is what the handler answers with, and podman's create errors
			// (name/size/driver) never contain the value — only the name,
			// which is public metadata.
			slog.ErrorContext(ctx, "secret creation failed",
				slog.Any("error", err),
				slog.String("userID", userID),
				slog.String("machineID", machineID),
				slog.String("secretName", req.Name))
			return appErrors.ErrSecretCreationFailed
		}
		if report == nil {
			slog.ErrorContext(ctx, "secret creation returned no report",
				slog.String("userID", userID),
				slog.String("machineID", machineID),
				slog.String("secretName", req.Name))
			return appErrors.ErrSecretCreationFailed
		}

		response = &SecretCreateResponse{ID: report.ID}
		return nil
	})
	return response, err
}

// Delete removes a secret (by name or ID) from the specified machine.
// Podman answers 204 with no body on success, and the API's response body
// is empty too — the frontend series (170) invalidates the list on the 204.
func (s *SecretService) Delete(ctx context.Context, userID, machineID, secretID string) error {
	return s.conn.WithConnection(ctx, userID, machineID, "secrets.Delete", func(connCtx context.Context) error {
		if err := podmanSecrets.Remove(connCtx, secretID); err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrSecretNotFound
			}
			if podmanerr.IsConflict(err) {
				// Surface podman's own refusal text (which container holds the
				// secret) — a 4xx, so the message reaches the frontend.
				return appErrors.ErrSecretInUse.WithMessage(podmanerr.Message(err))
			}
			return fmt.Errorf("failed to delete secret: %w", err)
		}

		return nil
	})
}
