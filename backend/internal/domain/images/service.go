package images

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/podland/backend/internal/connections"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// Compile-time check that *ImageService satisfies ImageServiceInterface.
var _ ImageServiceInterface = (*ImageService)(nil)

// concreteConnectionManager wraps a *connections.ConnectionManager to implement ConnectionManagerInterface.
// This is needed for backward compatibility.
type concreteConnectionManager struct {
	connManager *connections.ConnectionManager
}

// GetConnection implements ConnectionManagerInterface by calling the underlying ConnectionManager.
func (c *concreteConnectionManager) GetConnection(ctx context.Context, machineID, userID string) (any, error) {
	return c.connManager.GetConnection(ctx, machineID, userID)
}

func (c *concreteConnectionManager) Release(machineID string) {
	c.connManager.Release(machineID)
}

// ImageServiceInterface defines the interface for image services.
// This interface is used for dependency injection and testing.
// Method signatures will be added as slices 1–3 (List, Delete, Pull) are implemented.
type ImageServiceInterface interface {
	List(ctx context.Context, userID, machineID string, opts entities.ImageListOptions) ([]*entities.ImageSummary, error)
	Delete(ctx context.Context, userID, machineID, imageID string, force bool) error
	Pull(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error)
}

// ImageService provides business logic for images.
type ImageService struct {
	connManager machines.ConnectionManagerInterface
}

// NewImageService creates a new ImageService with the given connection manager interface.
func NewImageService(connManager machines.ConnectionManagerInterface) *ImageService {
	return &ImageService{
		connManager: connManager,
	}
}

// NewImageServiceWithConcreteDeps creates a new ImageService with concrete dependencies.
// This maintains backward compatibility with existing code that uses *connections.ConnectionManager.
func NewImageServiceWithConcreteDeps(connManager *connections.ConnectionManager) *ImageService {
	return NewImageService(&concreteConnectionManager{connManager: connManager})
}

// List returns a list of images on the specified machine.
func (s *ImageService) List(ctx context.Context, userID, machineID string, opts entities.ImageListOptions) ([]*entities.ImageSummary, error) {
	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}

	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	imageEngine := &tunnel.ImageEngine{
		ClientCtx: connCtx,
	}

	images, err := imageEngine.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	return images, nil
}

// Delete removes an image by ID on the specified machine.
// Stub — implemented in slice 060-02-image-delete.
func (s *ImageService) Delete(ctx context.Context, userID, machineID, imageID string, force bool) error {
	if ctx == nil {
		return fmt.Errorf("context is required")
	}
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	if machineID == "" {
		return fmt.Errorf("machineID is required")
	}
	if imageID == "" {
		return fmt.Errorf("imageID is required")
	}

	connCtxInterface, err := s.connManager.GetConnection(ctx, machineID, userID)
	if err != nil {
		return fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return fmt.Errorf("invalid connection type: expected context.Context")
	}

	imageEngine := &tunnel.ImageEngine{
		ClientCtx: connCtx,
	}

	report, errs := imageEngine.Remove(ctx, []string{imageID}, entities.ImageRemoveOptions{
		Force: force,
	})
	if err != nil {
		if strings.Contains(err.Error(), "no such image") || strings.Contains(err.Error(), "image not found") {
			return appErrors.ErrImageNotFound
		}
		if strings.Contains(err.Error(), "image is used by") || strings.Contains(err.Error(), "unable to delete") {
			return appErrors.ErrImageUsedByContainers
		}
		return fmt.Errorf("failed to remove image: %w", err)
	}

	if len(errs) > 0 && errs[0] != nil {
		if strings.Contains(errs[0].Error(), "no such image") || strings.Contains(errs[0].Error(), "image not found") {
			return appErrors.ErrImageNotFound
		}
		if strings.Contains(errs[0].Error(), "image is used by") || strings.Contains(errs[0].Error(), "unable to delete") {
			return appErrors.ErrImageUsedByContainers
		}
		return fmt.Errorf("failed to remove image: %w", errs[0])
	}

	if report == nil {
		return fmt.Errorf("no remove report returned for image: %s", imageID)
	}

	return nil
}

// Pull pulls an image on the specified machine.
// Stub — implemented in slice 060-03-image-pull-basic.
func (s *ImageService) Pull(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error) {
	return nil, appErrors.ErrNotImplemented
}
