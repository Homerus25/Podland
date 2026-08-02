package images

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	PullWithProgress(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions, progressWriter func(event PullProgressEvent)) (*entities.ImagePullReport, error)
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
func (s *ImageService) Pull(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error) {
	return s.PullWithProgress(ctx, userID, machineID, rawImage, opts, nil)
}

// PullWithProgress pulls an image on the specified machine and reports progress via the provided callback.
func (s *ImageService) PullWithProgress(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions, progressWriter func(event PullProgressEvent)) (*entities.ImagePullReport, error) {
	connCtxInterface, err := s.connManager.GetConnection(ctx, machineID, userID)
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

	// Set up progress writer if callback is provided
	if progressWriter != nil {
		opts.Writer = &progressAdapter{
			writer:     progressWriter,
			machineID:  machineID,
			operationID: generateOperationID(),
		}
	}

	report, err := imageEngine.Pull(ctx, rawImage, opts)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return nil, appErrors.ErrOperationCancelled
		}
		return nil, appErrors.ErrImagePullFailed
	}

	return report, nil
}

// progressAdapter adapts podman progress output to our progress event format
type progressAdapter struct {
	writer       func(event PullProgressEvent)
	machineID    string
	operationID  string
}

// Write implements io.Writer to capture podman progress output
func (p *progressAdapter) Write(data []byte) (int, error) {
	// For now, we'll emit basic progress events
	// The actual podman progress format may vary, so we emit a generic status update
	p.writer(NewProgressEvent(p.operationID, p.machineID, "pulling", string(data), 0, 0))
	return len(data), nil
}

// generateOperationID generates a unique operation ID for tracking pull operations
func generateOperationID() string {
	return fmt.Sprintf("pull-%d", time.Now().UnixNano())
}
