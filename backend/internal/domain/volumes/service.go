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
	Create(ctx context.Context, userID, machineID string, req VolumeCreateRequest) (*entities.VolumeInspectReport, error)
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

// Create makes a volume on the machine and returns its full config.
// An empty request name yields an anonymous volume; the returned config
// carries podman's generated name. Podman-side rejections (invalid name
// format, driver errors, already-existing name) surface as 400
// INVALID_REQUEST carrying podman's message, not 5xx.
func (s *VolumeService) Create(ctx context.Context, userID, machineID string, req VolumeCreateRequest) (*entities.VolumeInspectReport, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	volumeEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	report, err := volumeEngine.VolumeCreate(ctx, ToPodmanVolumeCreateOptions(req))
	if err != nil {
		if appErr := mapCreateError(err); appErr != nil {
			return nil, appErr
		}
		return nil, fmt.Errorf("failed to create volume: %w", err)
	}

	// Create only reports the name; the contract is the full config, so
	// inspect the freshly created volume by its (possibly generated) name.
	inspect := func(name string) (*entities.VolumeInspectReport, error) {
		reports, errs, err := volumeEngine.VolumeInspect(ctx, []string{name}, entities.InspectOptions{})
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
	return inspect(report.IDOrName)
}

// mapCreateError converts podman's client-side rejections into 4xx app
// errors. A 4xx ErrorModel from the podman service means the request
// itself was bad (unknown driver/option, name in use). Invalid NAME
// format is special: podman's compat layer rejects it as a 500
// ErrorModel (or a plain "invalid argument" error), so detection also
// rides on podman's validation text; everything else stays a wrapped 5xx.
func mapCreateError(err error) *appErrors.AppError {
	msg := err.Error()
	var errModel *errorhandling.ErrorModel
	if errors.As(err, &errModel) {
		msg = errModel.Message
		if errModel.ResponseCode >= 400 && errModel.ResponseCode < 500 {
			return appErrors.ErrInvalidRequest.WithMessage(msg)
		}
	}
	if strings.Contains(msg, "invalid argument") || strings.Contains(msg, "names must match") {
		return appErrors.ErrInvalidRequest.WithMessage(msg)
	}
	return nil
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
