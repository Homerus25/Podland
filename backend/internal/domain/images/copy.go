package images

import (
	"context"
	"errors"
	"fmt"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/permission"
	"github.com/Homerus25/podland/backend/internal/transfer"
)

// Cross-machine image copy (issue 375, architecture-decisions §19): the
// shared transfer kit owns the export→load splice; this file owns the
// images domain's composition around it — the preview's already-present
// fact, the copy's source existence gate, and the mapping onto the wire
// vocabulary. Copy-only: no source removal, no force, no mode. The full
// image ID is the wire's image reference; the archive preserves the
// image's tags, and the load report's names ride the terminal event.

// Transfer status label for the image copy endpoint (issue 375): copy
// answers "copied" — there is no move mode.
const ImageCopyStatusCopied = "copied"

// ImageCopyRequest is the body of the cross-machine image copy endpoint
// (POST .../images/{imageId}/copy, issue 375). Copy-only: no mode, no
// force — the source is never touched.
type ImageCopyRequest struct {
	// TargetMachineID is the machine the image is transferred to. Required.
	TargetMachineID string `json:"targetMachineId" validate:"required,min=1"`
}

// ImageCopyPreviewResponse answers the copy dialog's one fact (GET
// .../images/{imageId}/copy-preview, issue 375): whether the target
// machine already holds the image by ID. Information, never a block —
// the load is idempotent (content-addressed), so confirming stays
// possible either way.
type ImageCopyPreviewResponse struct {
	// Present reports whether the target machine already holds the
	// image by ID.
	Present bool `json:"present"`
}

// ImageCopyResponse is the transfer envelope the copy answers with
// (issue 375).
type ImageCopyResponse struct {
	// ID of the copied image (the full image ID — the wire's image
	// reference; identical on both machines).
	ID string `json:"id"`
	// Status of the transfer: "copied".
	Status string `json:"status"`
	// SourceMachineID is the machine the image was transferred from.
	SourceMachineID string `json:"sourceMachineId"`
	// TargetMachineID is the machine the image was loaded on.
	TargetMachineID string `json:"targetMachineId"`
	// Bytes is the archive actually streamed across machines.
	Bytes int64 `json:"bytes"`
	// Names are the image names the target's load reported (the tags
	// the load applied). Never nil — a load without names renders an
	// empty JSON array.
	Names []string `json:"names"`
}

// Stream-event type labels for the image copy's NDJSON transport
// (issue 375): progress events, then exactly one terminal event — the
// transfer envelope on success, the mapped error otherwise. The volume
// move's vocabulary (issue 238).
const (
	ImageCopyEventTypeImage    = "image"
	ImageCopyEventTypeComplete = "complete"
	ImageCopyEventTypeError    = "error"
)

// Progress phase labels for ImageCopyEvent.Phase (issue 375): "start"
// when the archive stream begins, "bytes" as the counting reader crosses
// its reporting increment, "done" when the load finished.
const (
	ImageCopyPhaseStart = "start"
	ImageCopyPhaseBytes = "bytes"
	ImageCopyPhaseDone  = "done"
)

// ImageCopyError carries an error code/message in a terminal copy event.
type ImageCopyError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ImageCopyEvent is one NDJSON event in the image copy stream (issue
// 375). Progress events carry Phase (plus Bytes where the phase has it);
// the terminal complete event carries the transfer envelope; the
// terminal error event carries the mapped code/message. All events
// share operationId, machineId, and timestamp — the volume move event
// shape (issue 238).
type ImageCopyEvent struct {
	OperationID string             `json:"operationId"`
	Type        string             `json:"type"`
	MachineID   string             `json:"machineId"`
	Image       string             `json:"image,omitempty"`
	Phase       string             `json:"phase,omitempty"`
	Bytes       int64              `json:"bytes,omitempty"`
	Result      *ImageCopyResponse `json:"result,omitempty"`
	Error       *ImageCopyError    `json:"error,omitempty"`
	Timestamp   string             `json:"timestamp"`
}

// NewImageCopyProgressEvent wraps one data-phase progress event.
func NewImageCopyProgressEvent(operationID, machineID, imageID, phase string, bytes int64) ImageCopyEvent {
	return ImageCopyEvent{
		OperationID: operationID,
		Type:        ImageCopyEventTypeImage,
		MachineID:   machineID,
		Image:       imageID,
		Phase:       phase,
		Bytes:       bytes,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// NewImageCopyCompleteEvent builds the terminal complete event carrying
// the transfer envelope.
func NewImageCopyCompleteEvent(operationID, machineID string, result ImageCopyResponse) ImageCopyEvent {
	return ImageCopyEvent{
		OperationID: operationID,
		Type:        ImageCopyEventTypeComplete,
		MachineID:   machineID,
		Result:      &result,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// NewImageCopyErrorEvent builds the terminal error event.
func NewImageCopyErrorEvent(operationID, machineID, code, message string) ImageCopyEvent {
	return ImageCopyEvent{
		OperationID: operationID,
		Type:        ImageCopyEventTypeError,
		MachineID:   machineID,
		Error:       &ImageCopyError{Code: code, Message: message},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// WithAccessRepo wires the machine-access store the cross-machine copy
// (issue 375) gates the target machine on, and returns the service for
// chaining. Production wiring always sets it; calling a copy path on a
// service without it is a wiring bug and panics at the gate.
func (s *ImageService) WithAccessRepo(accessRepo permission.PermissionGetter) *ImageService {
	s.accessRepo = accessRepo
	return s
}

// WithMaxImageTransferBytes wires the copy's archive stream bound
// (issue 375, §19) and returns the service for chaining. The bound is
// service-owned — the IMAGE_TRANSFER_MAX_BYTES config value — and zero
// falls back to the transfer kit's DefaultMaxImageTransferBytes (the kit
// owns the default per §19).
func (s *ImageService) WithMaxImageTransferBytes(max int64) *ImageService {
	s.maxImageTransferBytes = max
	return s
}

// imageTransferLimit answers the wired transfer bound; the zero-value
// fallback resolves through the transfer kit's ImageBoundOrDefault —
// the kit owns the default (§19), so this fallback and the config
// default cannot drift apart.
func (s *ImageService) imageTransferLimit() int64 {
	return transfer.ImageBoundOrDefault(s.maxImageTransferBytes)
}

// CopyPreview answers whether the target machine already holds the image
// by ID (issue 375, doku/description_images_api.md). The fact is
// information for the dialog, never a block: the load is idempotent
// (content-addressed), so a present target image does not refuse
// anything.
//
// Access: the route's subtree middleware already proved the caller holds
// an access row on the source machine — read level suffices, this
// endpoint mutates nothing. The target machine is a service-side gate
// (route middleware only sees the route's machine): the caller's stored
// level must be at least read, checked against the access store BEFORE
// any podman call, so a caller without target access gets the standard
// 403 ACCESS_DENIED and never a false preview.
func (s *ImageService) CopyPreview(ctx context.Context, userID, machineID, imageID, targetMachineID string) (*ImageCopyPreviewResponse, error) {
	// The target gate fires before the connection lease (the volumes
	// move-preview's shape): a lookup failure is 500, a level below
	// read — an absent row included — is 403, per
	// permission.RequireLevel's single-owner contract.
	if err := permission.RequireLevel(s.accessRepo, targetMachineID, userID, models.PermissionRead); err != nil {
		return nil, err
	}

	// Source lease: the image must exist (404) before the target is read.
	exists, err := s.imageExists(ctx, userID, machineID, imageID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, appErrors.ErrImageNotFound
	}

	// Target lease: the already-present fact, read by image ID through
	// the same inspect ritual — a not-found there is the fact (absent),
	// not an error.
	var present bool
	err = s.engines.WithEngine(ctx, userID, targetMachineID, "images.CopyPreviewTarget", func(e ImageEngine) error {
		present, err = imagePresent(e, imageID)
		return err
	})
	if err != nil {
		return nil, err
	}

	return &ImageCopyPreviewResponse{Present: present}, nil
}

// Copy transfers an image to another machine (issue 375, §19): the
// source's export spliced through the shared transfer kit into the
// target's load, with live byte progress. Copy-only — the source is
// never touched; there is no move mode, no force, no mode on the wire.
//
// Access: the route's write-level middleware proved the caller may
// mutate the SOURCE machine; the TARGET machine is a service-side gate
// (route middleware only sees the route's machine): the caller's stored
// level must be at least write, checked against the access store BEFORE
// any podman call.
//
// Copy answers through the NDJSON stream (the handler always streams);
// onProgress is the transport's feed.
func (s *ImageService) Copy(ctx context.Context, userID, machineID, imageID, targetMachineID string, onProgress func(ImageCopyEvent)) (*ImageCopyResponse, error) {
	// The target gate fires before any connection lease (the volumes
	// move's shape): a lookup failure is 500, a level below write — an
	// absent row included — is 403, per permission.RequireLevel's
	// single-owner contract.
	if err := permission.RequireLevel(s.accessRepo, targetMachineID, userID, models.PermissionWrite); err != nil {
		return nil, err
	}

	// Source lease: the image must exist (404) before anything streams.
	exists, err := s.imageExists(ctx, userID, machineID, imageID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, appErrors.ErrImageNotFound
	}

	// Data phase (§19): two connection leases stand open at once — the
	// export reads the source machine while the load writes the target
	// — so the nested WithEngine is deliberate, the volumes data
	// phase's arrangement. The kit's splice streams the archive across
	// with live byte progress.
	var response *ImageCopyResponse
	err = s.engines.WithEngine(ctx, userID, machineID, "images.CopySource", func(src ImageEngine) error {
		return s.engines.WithEngine(ctx, userID, targetMachineID, "images.CopyTarget", func(tgt ImageEngine) error {
			emit := func(phase string, bytes int64) {
				if onProgress != nil {
					// OperationID stays empty: the streaming handler
					// stamps its own (the volumes move's arrangement).
					onProgress(NewImageCopyProgressEvent("", machineID, imageID, phase, bytes))
				}
			}

			emit(ImageCopyPhaseStart, 0)
			count, names, err := transfer.CopyImageData(ctx, src, tgt, imageID, s.imageTransferLimit(), func(count int64) {
				emit(ImageCopyPhaseBytes, count)
			})
			if err == nil {
				emit(ImageCopyPhaseDone, count)
				response = &ImageCopyResponse{
					ID:              imageID,
					Status:          ImageCopyStatusCopied,
					SourceMachineID: machineID,
					TargetMachineID: targetMachineID,
					Bytes:           count,
					Names:           names,
				}
				return nil
			}

			// No rollback, by design: images are content-addressed, so
			// a failed load leaves already-transferred layers on the
			// target and a retry skips them (idempotence) — there is
			// no partial-state poisoning and no rollback machinery
			// (the volumes splice's rollback exists only because a
			// half-populated volume would block a retry with a false
			// collision; an image never does).
			if errors.Is(err, transfer.ErrBoundExceeded) {
				return appErrors.ErrImageTransferTooLarge.WithMessage(fmt.Sprintf(
					"Image %s exceeded the transfer bound at %d bytes", imageID, count))
			}
			if ctx.Err() != nil {
				// No narration rides this error, by design — the
				// handler streams through the kit whose cancelled-wins
				// rule answers a cancelled stream with the generic
				// OPERATION_CANCELLED terminal event whatever the
				// operation returns.
				return ctx.Err()
			}
			return appErrors.ErrImageTransferFailed.WithMessage(fmt.Sprintf("Image %s: %v", imageID, err))
		})
	})
	if err != nil {
		return nil, err
	}
	if response != nil && response.Names == nil {
		response.Names = []string{}
	}
	return response, nil
}

// imageExists answers whether the image exists on the machine through
// the image engine seam.
func (s *ImageService) imageExists(ctx context.Context, userID, machineID, imageID string) (bool, error) {
	exists := false
	err := s.engines.WithEngine(ctx, userID, machineID, "images.ImageExists", func(e ImageEngine) error {
		present, err := imagePresent(e, imageID)
		if err != nil {
			return err
		}
		exists = present
		return nil
	})
	return exists, err
}

// imagePresent answers whether the machine holds the image by ID through
// the shared single-report inspect ritual: the domain's not-found
// sentinel is the fact (absent), not an error.
func imagePresent(e ImageEngine, id string) (bool, error) {
	if _, err := inspectImage(e, id); err != nil {
		if errors.Is(err, appErrors.ErrImageNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// No adapter widens the images engine to the transfer kit's widths: the
// engine's Export/Load match the kit's ImageDataSource/ImageDataTarget
// method names, so it satisfies them structurally (§19's "adapters
// widen, never wrap" applies only where the names differ; here they do
// not).
