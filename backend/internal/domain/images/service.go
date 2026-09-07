package images

import (
	"context"
	"fmt"
	"time"

	"github.com/Homerus25/podland/backend/internal/connections"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// Compile-time check that *ImageService satisfies ImageServiceInterface.
var _ ImageServiceInterface = (*ImageService)(nil)

// ImageServiceInterface defines the interface for image services.
// This interface is used for dependency injection and testing.
// Method signatures will be added as slices 1–3 (List, Delete, Pull) are implemented.
type ImageServiceInterface interface {
	List(ctx context.Context, userID, machineID string, opts entities.ImageListOptions) ([]*entities.ImageSummary, error)
	Delete(ctx context.Context, userID, machineID, imageID string, force bool) error
	Pull(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error)
	PullWithProgress(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions, progressWriter func(event PullProgressEvent)) (*entities.ImagePullReport, error)
	Prune(ctx context.Context, userID, machineID string) ([]*reports.PruneReport, error)
}

// ImageService provides business logic for images.
type ImageService struct {
	conn connections.Manager
}

// NewImageService creates a new ImageService with the given connection manager.
func NewImageService(conn connections.Manager) *ImageService {
	return &ImageService{
		conn: conn,
	}
}

// List returns a list of images on the specified machine.
func (s *ImageService) List(ctx context.Context, userID, machineID string, opts entities.ImageListOptions) ([]*entities.ImageSummary, error) {
	var summaries []*entities.ImageSummary
	err := s.conn.WithConnection(ctx, userID, machineID, "images.List", func(connCtx context.Context) error {
		imageEngine := &tunnel.ImageEngine{
			ClientCtx: connCtx,
		}

		list, err := imageEngine.List(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to list images: %w", err)
		}
		summaries = list
		return nil
	})
	return summaries, err
}

// Delete removes an image by ID on the specified machine.
// Stub — implemented in slice 060-02-image-delete.
func (s *ImageService) Delete(ctx context.Context, userID, machineID, imageID string, force bool) error {
	return s.conn.WithConnection(ctx, userID, machineID, "images.Delete", func(connCtx context.Context) error {
		imageEngine := &tunnel.ImageEngine{
			ClientCtx: connCtx,
		}

		report, errs := imageEngine.Remove(ctx, []string{imageID}, entities.ImageRemoveOptions{
			Force: force,
		})

		if len(errs) > 0 && errs[0] != nil {
			// The adapter's conflict predicate is a superset of the pair of
			// phrases this slice used to match: podman also refuses via the
			// "in use" family and typed 409 ErrorModels.
			if podmanerr.IsNotFound(errs[0]) {
				return appErrors.ErrImageNotFound
			}
			if podmanerr.IsConflict(errs[0]) {
				return appErrors.ErrImageUsedByContainers
			}
			return fmt.Errorf("failed to remove image: %w", errs[0])
		}

		if report == nil {
			return fmt.Errorf("no remove report returned for image: %s", imageID)
		}

		return nil
	})
}

// Pull pulls an image on the specified machine.
func (s *ImageService) Pull(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error) {
	return s.PullWithProgress(ctx, userID, machineID, rawImage, opts, nil)
}

// PullWithProgress pulls an image on the specified machine and reports progress via the provided callback.
func (s *ImageService) PullWithProgress(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions, progressWriter func(event PullProgressEvent)) (*entities.ImagePullReport, error) {
	var report *entities.ImagePullReport
	err := s.conn.WithConnection(ctx, userID, machineID, "images.Pull", func(connCtx context.Context) error {
		imageEngine := &tunnel.ImageEngine{
			ClientCtx: connCtx,
		}

		// Set up progress writer if callback is provided
		if progressWriter != nil {
			opts.Writer = &progressAdapter{
				writer:      progressWriter,
				machineID:   machineID,
				operationID: generateOperationID(),
			}
		}

		pullReport, err := imageEngine.Pull(ctx, rawImage, opts)
		if err != nil {
			if ctx.Err() == context.Canceled {
				return appErrors.ErrOperationCancelled
			}
			return appErrors.ErrImagePullFailed
		}

		report = pullReport
		return nil
	})
	return report, err
}

// Prune removes every unused image on the machine in one call,
// tagged or not (the equivalent of `podman image prune -a`). Images
// in use by containers are untouched by podman. Returns one prune
// report per removed image with its reclaimed size.
func (s *ImageService) Prune(ctx context.Context, userID, machineID string) ([]*reports.PruneReport, error) {
	var pruned []*reports.PruneReport
	err := s.conn.WithConnection(ctx, userID, machineID, "images.Prune", func(connCtx context.Context) error {
		imageEngine := &tunnel.ImageEngine{
			ClientCtx: connCtx,
		}

		pruneReports, err := imageEngine.Prune(ctx, entities.ImagePruneOptions{
			All: true,
		})
		if err != nil {
			return fmt.Errorf("failed to prune images: %w", err)
		}
		pruned = pruneReports
		return nil
	})
	return pruned, err
}

// progressAdapter adapts podman progress output to our progress event format
type progressAdapter struct {
	writer      func(event PullProgressEvent)
	machineID   string
	operationID string
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
