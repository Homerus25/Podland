package networks

import (
	"context"
	"fmt"

	"go.podman.io/common/libnetwork/types"
)

// NetworkService provides business logic for networks. Networks live
// only in the remote machine's podman; this service proxies podman
// directly and persists nothing locally, matching VolumeService/
// SecretService. Every machine-scoped podman call crosses the
// NetworkEngineProvider (the connection seam); the service itself owns
// only domain mapping.
type NetworkService struct {
	engines NetworkEngineProvider
}

// NewNetworkService creates a new NetworkService
func NewNetworkService(engines NetworkEngineProvider) *NetworkService {
	return &NetworkService{
		engines: engines,
	}
}

// List returns every network on the specified machine — everything
// podman reports, including the default bridge network; no filtering.
func (s *NetworkService) List(ctx context.Context, userID, machineID string) ([]types.Network, error) {
	var networks []types.Network
	err := s.engines.WithEngine(ctx, userID, machineID, "networks.List", func(e NetworkEngine) error {
		n, err := e.List()
		if err != nil {
			return fmt.Errorf("failed to list networks: %w", err)
		}
		networks = n
		return nil
	})
	return networks, err
}
