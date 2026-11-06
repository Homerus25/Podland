package images

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/Homerus25/podland/backend/internal/bulkremove"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/inspect"
	"github.com/Homerus25/podland/backend/internal/permission"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// ImageService provides business logic for images. Every host-scoped
// podman call crosses the ImageEngineProvider (the connection seam); the
// service itself owns only domain mapping and progress conversion.
type ImageService struct {
	engines ImageEngineProvider
	// accessRepo resolves the caller's permission level for the
	// cross-host copy (issue 375): route middleware only sees the
	// route's host, so the target host's access is checked here,
	// against the store, before any podman call. Wired via
	// WithAccessRepo.
	accessRepo permission.PermissionGetter
	// maxImageTransferBytes bounds the cross-host copy's archive
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

// List returns a list of images on the specified host.
func (s *ImageService) List(ctx context.Context, userID, hostID string, opts entities.ImageListOptions) ([]*entities.ImageSummary, error) {
	var summaries []*entities.ImageSummary
	err := s.engines.WithEngine(ctx, userID, hostID, "images.List", func(e ImageEngine) error {
		list, err := e.List(opts)
		if err != nil {
			return fmt.Errorf("failed to list images: %w", err)
		}
		summaries = list
		return nil
	})
	return summaries, err
}

// Delete removes an image by ID on the specified host.
func (s *ImageService) Delete(ctx context.Context, userID, hostID, imageID string, force bool) error {
	return s.engines.WithEngine(ctx, userID, hostID, "images.Delete", func(e ImageEngine) error {
		report, errs := e.Remove(imageID, force)

		if len(errs) > 0 && errs[0] != nil {
			return classifyRemoveError(errs[0])
		}

		if report == nil {
			return fmt.Errorf("no remove report returned for image: %s", imageID)
		}

		return nil
	})
}

// classifyRemoveError maps one podman remove failure onto the domain's
// wire vocabulary — the single classification behind both removal
// surfaces (Delete's per-item error and the bulk report's per-batch
// errors), so the two cannot drift.
func classifyRemoveError(err error) error {
	// The adapter's conflict predicate is a superset of the pair of
	// phrases this slice used to match: podman also refuses via the
	// "in use" family and typed 409 ErrorModels.
	if podmanerr.IsNotFound(err) {
		return appErrors.ErrImageNotFound
	}
	if podmanerr.IsConflict(err) {
		return appErrors.ErrImageUsedByContainers
	}
	return fmt.Errorf("failed to remove image: %w", err)
}

// BulkDelete removes several images through ONE podman call — the batch
// remove endpoint accepts the whole array, so the bulk surface stops
// paying one podman call per id. The answer keeps the bulk-remove
// report contract (internal/bulkremove): one entry per requested id,
// in request order, a failure an entry and never a status code.
//
// Per-id attribution rides the batch report: Deleted and Untagged carry
// every reference the call removed (full image IDs land in Deleted, tag
// requests in Untagged), so an id the report shows removed answers
// success — a short-ID request included, which podman resolves to the
// full ID (imageRemovedBy). Podman's batch report carries an UNKEYED error
// slice — it cannot attribute an error to a requested id — so the
// failed ids answer one entry error: the sentinel every cause classifies
// to when they agree (the common batches: all in use, all vanished), the
// kit's generic internal entry otherwise, with the causes logged once
// and never surfaced. An acquisition failure (the lease never granted)
// answers the classified cause on every entry — byte-identical to the
// per-item loop's answer when every single-item call failed the same
// way (e.g. the host is down).
func (s *ImageService) BulkDelete(ctx context.Context, userID, hostID string, ids []string, force bool) []bulkremove.Entry {
	entries := make([]bulkremove.Entry, 0, len(ids))
	err := s.engines.WithEngine(ctx, userID, hostID, "images.BulkRemove", func(e ImageEngine) error {
		report, errs := e.RemoveAll(ids, force)
		entries = bulkRemoveEntries(ctx, ids, report, errs)
		return nil
	})
	if err != nil {
		for _, id := range ids {
			entries = append(entries, bulkremove.Entry{ID: id, Error: bulkremove.Classify(ctx, id, err)})
		}
	}
	return entries
}

// bulkRemoveEntries maps one batch remove call's report onto the
// per-id report entries: an id the report shows removed (Deleted or
// Untagged, short-ID requests included) answers success; the rest
// answer the batch's failure entry error. A nil report answers a
// call-level failure (transport or decode) — nothing was removed, so
// every id answers the classified cause.
func bulkRemoveEntries(ctx context.Context, ids []string, report *entities.ImageRemoveReport, errs []error) []bulkremove.Entry {
	entries := make([]bulkremove.Entry, len(ids))
	if report == nil {
		var cause error = fmt.Errorf("no remove report returned for bulk remove")
		if len(errs) > 0 && errs[0] != nil {
			cause = errs[0]
		}
		for i, id := range ids {
			entries[i] = bulkremove.Entry{ID: id, Error: bulkremove.Classify(ctx, id, cause)}
		}
		return entries
	}

	var failed []int
	for i, id := range ids {
		if imageRemovedBy(report, id) {
			entries[i] = bulkremove.Entry{ID: id, Success: true}
			continue
		}
		entries[i] = bulkremove.Entry{ID: id}
		failed = append(failed, i)
	}
	if len(failed) == 0 {
		return entries
	}

	entryErr := bulkFailureEntryError(ctx, errs)
	for _, i := range failed {
		entries[i].Error = entryErr
	}
	return entries
}

// imageRemovedBy reports whether the batch report shows the requested
// id removed. A full ID or tag answers by equality, but podman resolves
// a short hex ID to the full one, so a removed short-ID request lands in
// Deleted as the full sha256 ID — the prefix match keeps the batch from
// regressing a reference form the single-item delete accepts (it counts
// any non-error call as success) into a false failure entry.
func imageRemovedBy(report *entities.ImageRemoveReport, id string) bool {
	match := func(ref string) bool {
		return ref == id ||
			strings.HasPrefix(ref, id) ||
			strings.HasPrefix(ref, "sha256:"+id)
	}
	for _, ref := range report.Deleted {
		if match(ref) {
			return true
		}
	}
	for _, ref := range report.Untagged {
		if match(ref) {
			return true
		}
	}
	return false
}

// bulkFailureEntryError answers the one entry error every id the batch
// report did not remove carries. Podman's batch remove answers an
// unkeyed error slice — an error cannot be attributed to a requested id
// — so the failed ids share one entry: the sentinel every cause
// classifies to when they agree (the common batches: all in use, all
// vanished), the kit's generic internal entry otherwise. The causes are
// logged once and never surfaced; an id the report does not list while
// podman reports no error at all (an out-of-contract reference form)
// answers the generic entry too.
func bulkFailureEntryError(ctx context.Context, errs []error) *bulkremove.EntryError {
	sentinels := make(map[string]*appErrors.AppError)
	var causes []error
	for _, err := range errs {
		if err == nil {
			continue
		}
		causes = append(causes, err)
		if appErr, ok := errors.AsType[*appErrors.AppError](classifyRemoveError(err)); ok {
			sentinels[appErr.Code] = appErr
		}
	}
	if len(sentinels) == 1 {
		for _, appErr := range sentinels {
			return &bulkremove.EntryError{Code: appErr.Code, Message: appErr.Message}
		}
	}
	for _, cause := range causes {
		slog.ErrorContext(ctx, "bulk remove: unattributed per-image error",
			slog.Any("error", cause))
	}
	if len(causes) == 0 {
		slog.ErrorContext(ctx, "bulk remove: ids absent from the report with no error reported")
	}
	return &bulkremove.EntryError{Code: appErrors.ErrInternalError.Code, Message: appErrors.ErrInternalError.Message}
}

// Inspect returns the raw podman inspect report for one image on the
// specified host.
func (s *ImageService) Inspect(ctx context.Context, userID, hostID, imageID string) (*entities.ImageInspectReport, error) {
	var report *entities.ImageInspectReport
	err := s.engines.WithEngine(ctx, userID, hostID, "images.Inspect", func(e ImageEngine) error {
		inspectReport, err := inspectImage(e, imageID)
		if err != nil {
			return err
		}

		report = inspectReport
		return nil
	})
	return report, err
}

// Get returns one image's detail DTO on the specified host: the list
// fields combined with the inspect report's config fields. Unlike
// Inspect, the report is mapped to ImageDetailResponse here.
func (s *ImageService) Get(ctx context.Context, userID, hostID, imageID string) (ImageDetailResponse, error) {
	var detail ImageDetailResponse
	err := s.engines.WithEngine(ctx, userID, hostID, "images.Get", func(e ImageEngine) error {
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

// PullWithProgress pulls an image on the specified host and reports progress via the provided callback.
func (s *ImageService) PullWithProgress(ctx context.Context, userID, hostID, rawImage string, opts entities.ImagePullOptions, progressWriter func(event PullProgressEvent)) (*entities.ImagePullReport, error) {
	// Set up progress writer if callback is provided
	if progressWriter != nil {
		opts.Writer = &progressAdapter{
			writer: progressWriter,
			hostID: hostID,
		}
	}

	var report *entities.ImagePullReport
	err := s.engines.WithEngine(ctx, userID, hostID, "images.Pull", func(e ImageEngine) error {
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
// export's images twin). The image's existence is resolved first through
// podman's exists endpoint, so a missing ID fails with the clean 404
// before any byte is written — callers can still send error responses
// when this returns an AppError. Bytes flow straight from the remote
// podman into w with no server-side buffering and no size bound: the
// transfer bound exists to protect the copy's in-process pipe (§19), and
// here the browser consumes the stream directly — cancellation rides the
// request context.
func (s *ImageService) Export(ctx context.Context, userID, hostID, imageID string, w io.Writer) error {
	return s.engines.WithEngine(ctx, userID, hostID, "images.Export", func(e ImageEngine) error {
		// Existence gate: fail before writing anything to w.
		exists, err := e.Exists(imageID)
		if err != nil {
			return err
		}
		if !exists {
			return appErrors.ErrImageNotFound
		}

		return e.Export(imageID, w)
	})
}

// Prune removes every unused image on the host in one call,
// tagged or not (the equivalent of `podman image prune -a`). Images
// in use by containers are untouched by podman. Returns one prune
// report per removed image with its reclaimed size.
func (s *ImageService) Prune(ctx context.Context, userID, hostID string) ([]*reports.PruneReport, error) {
	var pruned []*reports.PruneReport
	err := s.engines.WithEngine(ctx, userID, hostID, "images.Prune", func(e ImageEngine) error {
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
	writer func(event PullProgressEvent)
	hostID string
}

// Write implements io.Writer to capture podman progress output
func (p *progressAdapter) Write(data []byte) (int, error) {
	// For now, we'll emit basic progress events
	// The actual podman progress format may vary, so we emit a generic status update
	p.writer(NewProgressEvent(p.hostID, "pulling", string(data), 0, 0))
	return len(data), nil
}
