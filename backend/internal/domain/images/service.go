package images

import (
	"context"
	"errors"
	"fmt"
	"io"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/inspect"
	"github.com/Homerus25/podland/backend/internal/permission"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// ImageService provides business logic for images. Every machine-scoped
// podman call crosses the ImageEngineProvider (the connection seam); the
// service itself owns only domain mapping and progress conversion.
type ImageService struct {
	engines ImageEngineProvider
	// accessRepo resolves the caller's permission level for the
	// cross-machine copy (issue 375): route middleware only sees the
	// route's machine, so the target machine's access is checked here,
	// against the store, before any podman call. Wired via
	// WithAccessRepo.
	accessRepo permission.PermissionGetter
	// maxImageTransferBytes bounds the cross-machine copy's archive
	// stream (issue 375, §19), enforced by the transfer kit's counting
	// reader. Wired via WithMaxImageTransferBytes from the
	// IMAGE_TRANSFER_MAX_BYTES config value; zero falls back to the
	// transfer kit's DefaultMaxImageTransferBytes (the kit owns the
	// default per §19).
	maxImageTransferBytes int64
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

// Inspect returns the raw podman inspect report for one image on the
// specified machine.
func (s *ImageService) Inspect(ctx context.Context, userID, machineID, imageID string) (*entities.ImageInspectReport, error) {
	var report *entities.ImageInspectReport
	err := s.engines.WithEngine(ctx, userID, machineID, "images.Inspect", func(e ImageEngine) error {
		inspectReport, err := inspectImage(e, imageID)
		if err != nil {
			return err
		}

		report = inspectReport
		return nil
	})
	return report, err
}

// Get returns one image's detail DTO on the specified machine: the list
// fields combined with the inspect report's config fields. Unlike
// Inspect, the report is mapped to ImageDetailResponse here.
func (s *ImageService) Get(ctx context.Context, userID, machineID, imageID string) (ImageDetailResponse, error) {
	var detail ImageDetailResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "images.Get", func(e ImageEngine) error {
		report, err := inspectImage(e, imageID)
		if err != nil {
			return err
		}

		detail = FromImageInspectReport(report)
		return nil
	})
	return detail, err
}

// inspectImage resolves one image's inspect report through the shared
// single-report ritual, classifying podman's not-found shapes into
// ErrImageNotFound.
func inspectImage(e ImageEngine, id string) (*entities.ImageInspectReport, error) {
	reports, inspectErrs, err := e.Inspect(id)
	return inspect.Resolve(reports, inspectErrs, err, appErrors.ErrImageNotFound, "image", id)
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
		// Cancellation is passed through, not mapped: the errors.Is
		// check below returns Cancellable's sentinel untouched — the
		// kit owns cancellation, and the run loop's cancelled-wins
		// rule answers OPERATION_CANCELLED whatever the operation
		// returns. A real pull failure still maps to
		// ErrImagePullFailed. Cancellable abandons the
		// pull goroutine inside this closure: the closure (and with it
		// the connection lease) returns promptly on a disconnect while
		// the pull keeps running untracked on the engine's detached
		// context until it ends naturally or the connection is reaped.
		// The abandoned goroutine's outcome lands in the helper's
		// buffered channel and is dropped; the report is propagated
		// only when the pull actually completed.
		var pullReport *entities.ImagePullReport
		pullErr := streaming.Cancellable(ctx, func() error {
			r, err := e.Pull(rawImage, opts)
			if err != nil {
				return err
			}
			pullReport = r
			return nil
		})
		if pullErr != nil {
			if errors.Is(pullErr, appErrors.ErrOperationCancelled) {
				return pullErr
			}
			return appErrors.ErrImagePullFailed
		}
		report = pullReport
		return nil
	})
	return report, err
}

// Export streams the image's tar archive into w (issue 401, the volume
// export's images twin). The image's existence is resolved first, so a
// missing ID fails with the clean 404 before any byte is written —
// callers can still send error responses when this returns an AppError.
// Bytes flow straight from the remote podman into w with no server-side
// buffering and no size bound: the transfer bound exists to protect the
// copy's in-process pipe (§19), and here the browser consumes the stream
// directly — cancellation rides the request context.
func (s *ImageService) Export(ctx context.Context, userID, machineID, imageID string, w io.Writer) error {
	return s.engines.WithEngine(ctx, userID, machineID, "images.Export", func(e ImageEngine) error {
		// Existence gate: fail before writing anything to w.
		if _, err := inspectImage(e, imageID); err != nil {
			return err
		}

		return e.Export(imageID, w)
	})
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
