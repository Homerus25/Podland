package images

import (
	"context"
	"fmt"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// ImageService provides business logic for images. Every machine-scoped
// podman call crosses the ImageEngineProvider (the connection seam); the
// service itself owns only domain mapping and progress conversion.
type ImageService struct {
	engines ImageEngineProvider
}

// NewImageService creates a new ImageService with the given engine provider.
func NewImageService(engines ImageEngineProvider) *ImageService {
	return &ImageService{
		engines: engines,
	}
}

// List returns a list of images on the specified machine.
func (s *ImageService) List(ctx context.Context, userID, machineID string, opts entities.ImageListOptions) ([]*entities.ImageSummary, error) {
	var summaries []*entities.ImageSummary
	err := s.engines.WithEngine(ctx, userID, machineID, "images.List", func(e ImageEngine) error {
		list, err := e.List(opts)
		if err != nil {
			return fmt.Errorf("failed to list images: %w", err)
		}
		summaries = list
		return nil
	})
	return summaries, err
}

// Delete removes an image by ID on the specified machine.
func (s *ImageService) Delete(ctx context.Context, userID, machineID, imageID string, force bool) error {
	return s.engines.WithEngine(ctx, userID, machineID, "images.Delete", func(e ImageEngine) error {
		report, errs := e.Remove(imageID, force)

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

// PullWithProgress pulls an image on the specified machine and reports progress via the provided callback.
func (s *ImageService) PullWithProgress(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions, progressWriter func(event PullProgressEvent)) (*entities.ImagePullReport, error) {
	// Set up progress writer if callback is provided
	if progressWriter != nil {
		opts.Writer = &progressAdapter{
			writer:    progressWriter,
			machineID: machineID,
		}
	}

	var report *entities.ImagePullReport
	err := s.engines.WithEngine(ctx, userID, machineID, "images.Pull", func(e ImageEngine) error {
		// Cancellation is not mapped here: the call crosses streaming
		// Run, whose cancelled-wins rule answers OPERATION_CANCELLED
		// whatever the operation returns.
		pullReport, err := e.Pull(rawImage, opts)
		if err != nil {
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
	err := s.engines.WithEngine(ctx, userID, machineID, "images.Prune", func(e ImageEngine) error {
		pruneReports, err := e.Prune()
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
	writer    func(event PullProgressEvent)
	machineID string
}

// Write implements io.Writer to capture podman progress output
func (p *progressAdapter) Write(data []byte) (int, error) {
	// For now, we'll emit basic progress events
	// The actual podman progress format may vary, so we emit a generic status update
	p.writer(NewProgressEvent(p.machineID, "pulling", string(data), 0, 0))
	return len(data), nil
}
