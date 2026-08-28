package secrets

import (
	"context"
	"fmt"

	"github.com/Homerus25/podland/backend/internal/domain/machines"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	podmanSecrets "go.podman.io/podman/v6/pkg/bindings/secrets"
)

// Compile-time check that *SecretService satisfies SecretServiceInterface.
var _ SecretServiceInterface = (*SecretService)(nil)

// SecretServiceInterface defines the interface for secret services.
// This interface is used for dependency injection and testing.
type SecretServiceInterface interface {
	List(ctx context.Context, userID, machineID string) ([]SecretListResponse, error)
}

// SecretService provides business logic for secrets. Secrets live only in
// the remote machine's podman; this service proxies podman directly and
// persists nothing locally, matching ContainerService/ImageService/
// QuadletService.
type SecretService struct {
	connManager machines.ConnectionManagerInterface
}

// NewSecretService creates a new SecretService with the given connection
// manager interface.
func NewSecretService(connManager machines.ConnectionManagerInterface) *SecretService {
	return &SecretService{
		connManager: connManager,
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
