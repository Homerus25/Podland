package images

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/web"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// ImageHandler handles image requests
//
// JSON endpoints cross the endpoint envelope (internal/web). Pull streams
// NDJSON progress through the streaming kit's progress seam
// (internal/streaming): HandleProgress owns the handler-side ritual —
// operationID, write deadline, slog line, headers, identity, disconnect,
// and error mapping.
type ImageHandler struct {
	service *ImageService
}

// NewImageHandler creates a new ImageHandler
func NewImageHandler(service *ImageService) *ImageHandler {
	return &ImageHandler{
		service: service,
	}
}

// List handles listing images
func (h *ImageHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]ImageListResponse, error) {
		opts := entities.ImageListOptions{}
		opts.All = web.QueryBool(r, "all", false)

		if filtersParam := r.URL.Query().Get("filters"); filtersParam != "" {
			var filters map[string][]string
			if err := json.Unmarshal([]byte(filtersParam), &filters); err != nil {
				slog.WarnContext(ctx, "invalid filters query param, ignoring", slog.String("filters", filtersParam), slog.Any("error", err))
			} else {
				for key, values := range filters {
					for _, value := range values {
						opts.Filter = append(opts.Filter, key+"="+value)
					}
				}
			}
		}

		images, err := h.service.List(ctx, userID, machineID, opts)
		if err != nil {
			return nil, err
		}
		return ToImageListResponse(images), nil
	})
}

// Pull handles pulling an image with streaming progress. Decode answers
// 400s before the stream begins; the streaming kit's progress seam
// (HandleProgress) owns the rest of the handler-side ritual.
func (h *ImageHandler) Pull(w http.ResponseWriter, r *http.Request) {
	var request ImagePullRequest
	if appErr := web.Decode(r, &request); appErr != nil {
		errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
		return
	}

	// Prepare pull options
	var pullOpts entities.ImagePullOptions
	if request.Credentials != nil {
		pullOpts.Username = request.Credentials.Username
		pullOpts.Password = request.Credentials.Password
	}
	if request.Platform != "" {
		os, arch, variant := ParsePlatform(request.Platform)
		pullOpts.OS = os
		pullOpts.Arch = arch
		pullOpts.Variant = variant
	}

	streaming.HandleProgress(w, r, "pull", []slog.Attr{slog.Any("request", request)}, streaming.ProgressConfig[PullProgressEvent, *entities.ImagePullReport]{
		Operation: func(pullCtx context.Context, op streaming.OperationContext, onProgress func(PullProgressEvent)) (*entities.ImagePullReport, error) {
			return h.service.PullWithProgress(pullCtx, op.UserID, op.MachineID, request.Image, pullOpts, func(event PullProgressEvent) {
				event.OperationID = op.OperationID
				event.MachineID = op.MachineID
				onProgress(event)
			})
		},
		CompleteEvent: func(op streaming.OperationContext, _ *entities.ImagePullReport) any {
			return NewCompleteEvent(op.OperationID, op.MachineID)
		},
		ErrorDefault: errors.ErrImagePullFailed,
		ErrorEvent: func(op streaming.OperationContext, code, message string, _ bool) any {
			return NewErrorEvent(op.OperationID, op.MachineID, code, message)
		},
	})
}

// Prune handles removing all unused images on a machine. Route-level
// RequireMachinePermission(write) (app.go) gates it at write; podman
// leaves in-use images alone. Returns 200 with one report entry per
// removed image (id, reclaimed size, any per-image error).
func (h *ImageHandler) Prune(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]ImagePruneResponse, error) {
		prunes, err := h.service.Prune(ctx, userID, machineID)
		if err != nil {
			return nil, err
		}
		return FromImagePruneReports(prunes), nil
	})
}

// Delete handles deleting an image. A malformed force flag is ignored
// (deletes as unforced), matching the documented behavior.
func (h *ImageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, machineID string) (struct{}, error) {
		imageID, guard := web.URLParam(r, "imageId", "MISSING_IMAGE_ID")
		if guard != nil {
			return struct{}{}, guard
		}

		force := web.QueryBool(r, "force", false)

		return struct{}{}, h.service.Delete(ctx, userID, machineID, imageID, force)
	})
}

// Get handles returning one image's detail DTO: the list fields
// combined with the inspect report's config fields. Read level (app.go
// mounts it without requireWrite).
func (h *ImageHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (ImageDetailResponse, error) {
		imageID, guard := web.URLParam(r, "imageId", "MISSING_IMAGE_ID")
		if guard != nil {
			return ImageDetailResponse{}, guard
		}

		return h.service.Get(ctx, userID, machineID, imageID)
	})
}

// Inspect handles returning the raw podman inspect report for one image.
// Read level (app.go mounts it without requireWrite); the report crosses
// the envelope as-is — no DTO mapping.
func (h *ImageHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*entities.ImageInspectReport, error) {
		imageID, guard := web.URLParam(r, "imageId", "MISSING_IMAGE_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.Inspect(ctx, userID, machineID, imageID)
	})
}

// CopyPreview answers the copy dialog's one fact (issue 375,
// doku/description_images_api.md): whether the target machine already
// holds the image by ID. Route-level RequireMachineAccess (app.go) gates
// the subtree at read; the target machine's read access is enforced
// inside the service against the access store.
func (h *ImageHandler) CopyPreview(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*ImageCopyPreviewResponse, error) {
		imageID, guard := web.URLParam(r, "imageId", "MISSING_IMAGE_ID")
		if guard != nil {
			return nil, guard
		}

		targetMachineID, guard := web.QueryString(r, "targetMachineId", "MISSING_TARGET_MACHINE_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.CopyPreview(ctx, userID, machineID, imageID, targetMachineID)
	})
}

// Copy handles the cross-machine copy of an image (issue 375,
// doku/description_images_api.md). The route sits at the subtree's write
// gate for the SOURCE machine; the target machine's write-level access
// is enforced inside the service against the access store.
//
// Transport: ALWAYS NDJSON — no plain/stream fork — through the
// streaming kit's progress seam (HandleProgress), the volumes move's
// arrangement. The archive stream can run minutes on real images, so
// the caller gets progress events and one terminal event (complete
// envelope or mapped error); the kit's 10-minute write deadline covers
// the stream's routine excess over the global 30-second write timeout.
// Errors answered BEFORE the stream begins (bad param, bad body) keep
// the plain envelope.
func (h *ImageHandler) Copy(w http.ResponseWriter, r *http.Request) {
	imageID, guard := web.URLParam(r, "imageId", "MISSING_IMAGE_ID")
	if guard != nil {
		errors.RespondWithError(w, guard, contextutils.GetRequestID(r.Context()))
		return
	}

	// Decode answers its 400s before the stream begins; the streaming
	// kit's progress seam takes over from there (the volumes move's
	// arrangement).
	var request ImageCopyRequest
	if appErr := web.Decode(r, &request); appErr != nil {
		errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
		return
	}

	streaming.HandleProgress(w, r, "Copy image across machines", []slog.Attr{slog.String("imageId", imageID)}, streaming.ProgressConfig[ImageCopyEvent, *ImageCopyResponse]{
		Operation: func(ctx context.Context, op streaming.OperationContext, onProgress func(ImageCopyEvent)) (*ImageCopyResponse, error) {
			return h.service.Copy(ctx, op.UserID, op.MachineID, imageID, request.TargetMachineID, func(event ImageCopyEvent) {
				event.OperationID = op.OperationID
				event.MachineID = op.MachineID
				onProgress(event)
			})
		},
		CompleteEvent: func(op streaming.OperationContext, response *ImageCopyResponse) any {
			return NewImageCopyCompleteEvent(op.OperationID, op.MachineID, *response)
		},
		ErrorDefault: errors.ErrConnectionFailed,
		ErrorEvent: func(op streaming.OperationContext, code, message string, _ bool) any {
			return NewImageCopyErrorEvent(op.OperationID, op.MachineID, code, message)
		},
	})
}
