package images

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

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
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) ([]ImageListResponse, error) {
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

		images, err := h.service.List(ctx, userID, hostID, opts)
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
	if appErr := web.Decode(w, r, &request); appErr != nil {
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
			return h.service.PullWithProgress(pullCtx, op.UserID, op.HostID, request.Image, pullOpts, func(event PullProgressEvent) {
				event.OperationID = op.OperationID
				event.HostID = op.HostID
				onProgress(event)
			})
		},
		CompleteEvent: func(op streaming.OperationContext, _ *entities.ImagePullReport) any {
			return NewCompleteEvent(op.OperationID, op.HostID)
		},
		ErrorDefault: errors.ErrImagePullFailed,
		ErrorEvent: func(op streaming.OperationContext, code, message string, _ bool) any {
			return NewErrorEvent(op.OperationID, op.HostID, code, message)
		},
	})
}

// Prune handles removing all unused images on a host. Route-level
// RequireHostPermission(write) (app.go) gates it at write; podman
// leaves in-use images alone. Returns 200 with one report entry per
// removed image (id, reclaimed size, any per-image error).
func (h *ImageHandler) Prune(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) ([]ImagePruneResponse, error) {
		prunes, err := h.service.Prune(ctx, userID, hostID)
		if err != nil {
			return nil, err
		}
		return FromImagePruneReports(prunes), nil
	})
}

// Delete handles deleting an image. A malformed force flag is ignored
// (deletes as unforced), matching the documented behavior.
func (h *ImageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, hostID string) (struct{}, error) {
		imageID, guard := web.URLParam(r, "imageId", "MISSING_IMAGE_ID")
		if guard != nil {
			return struct{}{}, guard
		}

		force := web.QueryBool(r, "force", false)

		return struct{}{}, h.service.Delete(ctx, userID, hostID, imageID, force)
	})
}

// Get handles returning one image's detail DTO: the list fields
// combined with the inspect report's config fields. Read level (app.go
// mounts it without requireWrite).
func (h *ImageHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (ImageDetailResponse, error) {
		imageID, guard := web.URLParam(r, "imageId", "MISSING_IMAGE_ID")
		if guard != nil {
			return ImageDetailResponse{}, guard
		}

		return h.service.Get(ctx, userID, hostID, imageID)
	})
}

// Inspect handles returning the raw podman inspect report for one image.
// Read level (app.go mounts it without requireWrite); the report crosses
// the envelope as-is — no DTO mapping.
func (h *ImageHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*entities.ImageInspectReport, error) {
		imageID, guard := web.URLParam(r, "imageId", "MISSING_IMAGE_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.Inspect(ctx, userID, hostID, imageID)
	})
}

// Export handles streaming an image's tar archive as a download (issue
// 401, the volume export's images twin). Read-level permission suffices
// (subtree RequireHostAccess): a download mutates nothing. It crosses
// the endpoint envelope's binary twin (web.HandleStream): identity
// guards, AppError mapping, and the 500 fallback live there; the closure
// is the tar headers plus the streaming service call. Headers are set
// before the service streams, so the download starts with the right
// Content-Type/Disposition; the service resolves existence first, so a
// 404 happens before any byte is written and the error envelope stays
// intact — the envelope's error write replaces the tar Content-Type.
// Mid-stream failures can't retroactively change the status — the
// connection just truncates. The filename names the short image ID (any
// sha256: prefix stripped, truncated to 12 hex characters) with a .tar
// suffix: the ID is the wire's image reference (the copy's established
// vocabulary), and a tag-based filename would need extra resolution and
// fails for untagged images. A pre-stream failure (the service's 404
// gate) drops the attachment disposition before the envelope writes the
// error, so a browser never offers the error JSON as a .tar download;
// after the first streamed byte the header is already flushed and the
// drop is a no-op.
func (h *ImageHandler) Export(w http.ResponseWriter, r *http.Request) {
	web.HandleStream(w, r, func(ctx context.Context, userID, hostID string) error {
		imageID, guard := web.URLParam(r, "imageId", "MISSING_IMAGE_ID")
		if guard != nil {
			return guard
		}

		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", shortImageID(imageID)+".tar"))

		if err := h.service.Export(ctx, userID, hostID, imageID, w); err != nil {
			w.Header().Del("Content-Disposition")
			return err
		}
		return nil
	})
}

// shortImageID derives the download filename's ID part: any sha256:
// prefix stripped, truncated to 12 hex characters. A shorter reference
// (never a real image ID, but the param is caller-chosen) passes through
// unchanged rather than panicking before the service's 404 gate.
func shortImageID(id string) string {
	digest := strings.TrimPrefix(id, "sha256:")
	if len(digest) > 12 {
		digest = digest[:12]
	}
	return digest
}

// CopyPreview answers the copy dialog's one fact (issue 375,
// doku/description_images_api.md): whether the target host already
// holds the image by ID. Route-level RequireHostAccess (app.go) gates
// the subtree at read; the target host's read access is enforced
// inside the service against the access store.
func (h *ImageHandler) CopyPreview(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*ImageCopyPreviewResponse, error) {
		imageID, guard := web.URLParam(r, "imageId", "MISSING_IMAGE_ID")
		if guard != nil {
			return nil, guard
		}

		targetHostID, guard := web.QueryString(r, "targetHostId", "MISSING_TARGET_HOST_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.CopyPreview(ctx, userID, hostID, imageID, targetHostID)
	})
}

// Copy handles the cross-host copy of an image (issue 375,
// doku/description_images_api.md). The route sits at the subtree's write
// gate for the SOURCE host; the target host's write-level access
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
	if appErr := web.Decode(w, r, &request); appErr != nil {
		errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
		return
	}

	streaming.HandleProgress(w, r, "Copy image across hosts", []slog.Attr{slog.String("imageId", imageID)}, streaming.ProgressConfig[ImageCopyEvent, *ImageCopyResponse]{
		Operation: func(ctx context.Context, op streaming.OperationContext, onProgress func(ImageCopyEvent)) (*ImageCopyResponse, error) {
			return h.service.Copy(ctx, op.UserID, op.HostID, imageID, request.TargetHostID, func(event ImageCopyEvent) {
				event.OperationID = op.OperationID
				event.HostID = op.HostID
				onProgress(event)
			})
		},
		CompleteEvent: func(op streaming.OperationContext, response *ImageCopyResponse) any {
			return NewImageCopyCompleteEvent(op.OperationID, op.HostID, *response)
		},
		ErrorDefault: errors.ErrConnectionFailed,
		ErrorEvent: func(op streaming.OperationContext, code, message string, _ bool) any {
			return NewImageCopyErrorEvent(op.OperationID, op.HostID, code, message)
		},
	})
}
