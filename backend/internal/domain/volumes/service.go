package volumes

import (
	"context"
	"fmt"

	"github.com/Homerus25/podland/backend/internal/domain/machines"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// VolumeServiceInterface defines the interface for volume services
// This interface is used for dependency injection and testing
type VolumeServiceInterface interface {
	List(ctx context.Context, userID, machineID string, opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error)
}

// VolumeService provides business logic for volumes
type VolumeService struct {
	connManager machines.ConnectionManagerInterface
}

// NewVolumeService creates a new VolumeService
func NewVolumeService(connManager machines.ConnectionManagerInterface) *VolumeService {
	return &VolumeService{
		connManager: connManager,
	}
}

// List returns every volume on the specified machine, named and anonymous
func (s *VolumeService) List(ctx context.Context, userID, machineID string, opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	volumeEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	volumes, err := volumeEngine.VolumeList(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}

	return volumes, nil
}
