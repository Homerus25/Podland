package volumes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Homerus25/podland/backend/internal/domain/machines"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// VolumeServiceInterface defines the interface for volume services
// This interface is used for dependency injection and testing
type VolumeServiceInterface interface {
	List(ctx context.Context, userID, machineID string, opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error)
	Get(ctx context.Context, userID, machineID, volumeName string) (*entities.VolumeInspectReport, error)
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

// Get returns the full configuration of a single volume by exact name
func (s *VolumeService) Get(ctx context.Context, userID, machineID, volumeName string) (*entities.VolumeInspectReport, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	volumeEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	reports, errs, err := volumeEngine.VolumeInspect(ctx, []string{volumeName}, entities.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect volume: %w", err)
	}
	for _, itemErr := range errs {
		if itemErr != nil {
			if isVolumeNotFound(itemErr) {
				return nil, appErrors.ErrVolumeNotFound
			}
			return nil, fmt.Errorf("failed to inspect volume: %w", itemErr)
		}
	}
	if len(reports) == 0 || reports[0] == nil {
		return nil, appErrors.ErrVolumeNotFound
	}
	return reports[0], nil
}

// isVolumeNotFound reports whether an item error from tunnel
// VolumeInspect means the volume does not exist. The tunnel layer
// converts a 404 ErrorModel from the podman service into a plain
// "no such volume %q" error (pkg/domain/infra/tunnel/volumes.go), so
// detection rides on that text; podman ErrorModels that reach us
// directly are still recognized via ResponseCode == 404.
func isVolumeNotFound(err error) bool {
	if strings.Contains(strings.ToLower(err.Error()), "no such volume") {
		return true
	}
	var errModel *errorhandling.ErrorModel
	return errors.As(err, &errModel) && errModel.ResponseCode == http.StatusNotFound
}
