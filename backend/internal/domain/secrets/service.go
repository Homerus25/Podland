package secrets

import (
	"context"
	"fmt"
	"log/slog"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/permission"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
)

// SecretService provides business logic for secrets. Secrets live only in
// the remote machine's podman; this service proxies podman directly and
// persists nothing locally, matching ContainerService/ImageService/
// QuadletService. Every machine-scoped podman call crosses the
// SecretEngineProvider (the connection seam); the service itself owns only
// domain mapping and the reveal gate.
type SecretService struct {
	engines SecretEngineProvider
	// usageEngines resolves the quadlet surface the cross-machine
	// copy/move preview (issue 239) derives its referencing-quadlet
	// warning from — the kit's machine-wide quadlet scan (issue 260,
	// §19) reads it through the provider's transfer.QuadletReader.
	// Wired via WithUsageEngines; production wiring always sets it —
	// calling a move path on a service without it is a wiring bug and
	// panics at the seam.
	usageEngines SecretUsageEngineProvider
	// accessRepo resolves the caller's permission level for the
	// admin-only value reveal (?reveal=true): the gate is a query param,
	// not a separate route, so it cannot ride on route-level middleware.
	// The cross-machine copy/move (issue 239) gates the source's reveal
	// and the target machine's level on it too.
	accessRepo permission.PermissionGetter
}

// NewSecretService creates a new SecretService with the given engine
// provider and machine-access repository (permission lookup for the
// admin-only reveal).
func NewSecretService(engines SecretEngineProvider, accessRepo permission.PermissionGetter) *SecretService {
	return &SecretService{
		engines:    engines,
		accessRepo: accessRepo,
	}
}

// WithUsageEngines wires the quadlet surface seam the cross-machine
// copy/move preview (issue 239) reads — the kit's quadlet scan (issue
// 260, §19) through the provider's transfer.QuadletReader — and
// returns the service for chaining. Production wiring always sets it;
// calling a move path on a service without it is a wiring bug and
// panics at the seam.
func (s *SecretService) WithUsageEngines(usageEngines SecretUsageEngineProvider) *SecretService {
	s.usageEngines = usageEngines
	return s
}

// List returns the metadata of the secrets on the specified machine.
func (s *SecretService) List(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
	var response []SecretListResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "secrets.List", func(e SecretEngine) error {
		reports, err := e.List()
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
	// row exists; the re-read of the LEVEL (so revocations and upgrades
	// take effect immediately) and the 500/403 decision belong to
	// permission.RequireLevel, the gate's single owner.
	if reveal {
		if err := permission.RequireLevel(s.accessRepo, machineID, userID, models.PermissionAdmin); err != nil {
			return nil, err
		}
	}

	var response *SecretInspectResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "secrets.Get", func(e SecretEngine) error {
		report, err := e.Inspect(secretID, reveal)
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
// secret's value as the raw request body reader (wired inside the engine
// adapter) and the name in options, so the value reaches podman
// byte-identical — never trimmed — under the file driver with no labels
// (v1 ruling: driver/labels deferred). The response carries only the
// created ID. A duplicate name surfaces as ErrSecretAlreadyExists (podman
// reports the conflict as a 500 ErrorModel whose cause carries the common
// package's "secret name in use" sentinel, classified by the adapter's
// cause check); any other creation failure becomes
// ErrSecretCreationFailed. The value itself never appears in any
// log statement on this path.
func (s *SecretService) Create(ctx context.Context, userID, machineID string, req SecretCreateRequest) (*SecretCreateResponse, error) {
	var response *SecretCreateResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "secrets.Create", func(e SecretEngine) error {
		report, err := e.Create(req.Name, req.Value)
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
	return s.engines.WithEngine(ctx, userID, machineID, "secrets.Delete", func(e SecretEngine) error {
		if err := e.Remove(secretID); err != nil {
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
