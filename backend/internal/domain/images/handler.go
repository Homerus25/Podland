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
	"github.com/google/uuid"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// ImageHandler handles image requests
//
// JSON endpoints cross the endpoint envelope (internal/web). Pull streams
// NDJSON progress through the streaming envelope (internal/streaming):
// Begin + Run own headers, identity, disconnect, and error mapping.
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

// Pull handles pulling an image with streaming progress
func (h *ImageHandler) Pull(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Pull")

	// Decode answers 400s before the stream begins; the streaming
	// envelope takes over from there.
	var request ImagePullRequest
	if appErr := web.Decode(r, &request); appErr != nil {
		errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
		return
	}

	slog.InfoContext(r.Context(), "pull", slog.Any("request", request))

	// Generate a unique operation ID for this pull
	operationID := uuid.New().String()

	stream := streaming.Begin(w, r)
	if stream == nil {
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

	streaming.Run(r.Context(), stream.Writer, streaming.StreamConfig[PullProgressEvent, *entities.ImagePullReport]{
		Operation: func(pullCtx context.Context, onProgress func(PullProgressEvent)) (*entities.ImagePullReport, error) {
			return h.service.PullWithProgress(pullCtx, stream.UserID, stream.MachineID, request.Image, pullOpts, func(event PullProgressEvent) {
				event.OperationID = operationID
				event.MachineID = stream.MachineID
				onProgress(event)
			})
		},
		OnEvent: func(event PullProgressEvent) error {
			return stream.Writer.Write(event)
		},
		CompleteEvent: func(*entities.ImagePullReport) any {
			return NewCompleteEvent(operationID, stream.MachineID)
		},
		ErrorDefault: errors.ErrImagePullFailed,
		ErrorEvent: func(code, message string, _ bool) any {
			return NewErrorEvent(operationID, stream.MachineID, code, message)
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
