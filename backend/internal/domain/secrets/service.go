package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/domain/machines"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	podmanSecrets "go.podman.io/podman/v6/pkg/bindings/secrets"
	"go.podman.io/podman/v6/pkg/errorhandling"
)

// Compile-time check that *SecretService satisfies SecretServiceInterface.
var _ SecretServiceInterface = (*SecretService)(nil)

// SecretServiceInterface defines the interface for secret services.
// This interface is used for dependency injection and testing.
type SecretServiceInterface interface {
	List(ctx context.Context, userID, machineID string) ([]SecretListResponse, error)
	Get(ctx context.Context, userID, machineID, secretID string, reveal bool) (*SecretInspectResponse, error)
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
	connManager machines.ConnectionManagerInterface
	// accessRepo resolves the caller's permission level for the
	// admin-only value reveal (?reveal=true): the gate is a query param,
	// not a separate route, so it cannot ride on route-level middleware.
	accessRepo accessPermissionGetter
}

// NewSecretService creates a new SecretService with the given connection
// manager interface and machine-access repository (permission lookup for
// the admin-only reveal).
func NewSecretService(connManager machines.ConnectionManagerInterface, accessRepo accessPermissionGetter) *SecretService {
	return &SecretService{
		connManager: connManager,
		accessRepo:  accessRepo,
	}
}

// List returns the metadata of the secrets on the specified machine.
func (s *SecretService) List(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)

	reports, err := podmanSecrets.List(connCtx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	return ToSecretListResponse(reports), nil
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

	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)

	opts := new(podmanSecrets.InspectOptions)
	if reveal {
		opts = opts.WithShowSecret(true)
	}

	report, err := podmanSecrets.Inspect(connCtx, secretID, opts)
	if err != nil {
		if isPodmanNotFound(err) {
			return nil, appErrors.ErrSecretNotFound
		}
		return nil, fmt.Errorf("failed to inspect secret: %w", err)
	}

	if reveal {
		// Audit event per the redaction discipline: who revealed what,
		// never the value itself.
		slog.InfoContext(ctx, "secret value revealed",
			slog.String("userID", userID),
			slog.String("machineID", machineID),
			slog.String("secretID", secretID))
	}

	return ToSecretInspectResponse(report, reveal), nil
}

// isPodmanNotFound reports whether err is podman's typed not-found answer.
// The bindings decode a non-2xx service API response body into an
// *errorhandling.ErrorModel carrying the HTTP status, so detection rides
// on ResponseCode == 404 — mirroring the tunnel layer (pkg/domain/infra/
// tunnel/secrets.go) instead of matching on message strings.
func isPodmanNotFound(err error) bool {
	var errModel *errorhandling.ErrorModel
	if errors.As(err, &errModel) {
		return errModel.ResponseCode == http.StatusNotFound
	}
	return false
}
