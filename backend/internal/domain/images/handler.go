package images

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// ImageHandler handles image requests
//
// JSON endpoints cross the endpoint envelope (internal/web). Pull
// keeps its own shape — it streams NDJSON progress via streaming.Run.
type ImageHandler struct {
	service ImageServiceInterface
}

// NewImageHandler creates a new ImageHandler
func NewImageHandler(service ImageServiceInterface) *ImageHandler {
	return &ImageHandler{
		service: service,
	}
}

// List handles listing images
func (h *ImageHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]ImageListResponse, error) {
		opts := entities.ImageListOptions{}

		if allParam := r.URL.Query().Get("all"); allParam != "" {
			if all, err := strconv.ParseBool(allParam); err == nil {
				opts.All = all
			}
		}

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

	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), errors.ErrInvalidCredentials.Code)
		return
	}

	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	var request ImagePullRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "Invalid request body", slog.Any("error", err))
		return
	}

	validate := validator.New()
	if err := validate.Struct(request); err != nil {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "Validation failed", slog.Any("error", err))
		return
	}

	slog.InfoContext(r.Context(), "pull", slog.Any("request", request))

	// Generate a unique operation ID for this pull
	operationID := uuid.New().String()

	// Set headers for streaming NDJSON response
	streaming.SetNDJSONHeaders(w)

	// Create NDJSON writer
	ndjsonWriter, err := streaming.NewNDJSONWriter(w)
	if err != nil {
		errors.RespondWithHTTPError(w, http.StatusInternalServerError, "STREAMING_NOT_SUPPORTED", "Streaming not supported", contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "Streaming not supported")
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

	streaming.Run(r.Context(), ndjsonWriter, streaming.StreamConfig[PullProgressEvent, *entities.ImagePullReport]{
		Operation: func(pullCtx context.Context, onProgress func(PullProgressEvent)) (*entities.ImagePullReport, error) {
			return h.service.PullWithProgress(pullCtx, userID, machineID, request.Image, pullOpts, func(event PullProgressEvent) {
				event.OperationID = operationID
				event.MachineID = machineID
				onProgress(event)
			})
		},
		OnEvent: func(event PullProgressEvent) error {
			return ndjsonWriter.Write(event)
		},
		CompleteEvent: func(*entities.ImagePullReport) any {
			return NewCompleteEvent(operationID, machineID)
		},
		ErrorEvent: func(err error, cancelled bool) any {
			if cancelled {
				return NewErrorEvent(operationID, machineID, errors.ErrOperationCancelled.Code, errors.ErrOperationCancelled.Message)
			}
			if appErr, ok := err.(*errors.AppError); ok {
				return NewErrorEvent(operationID, machineID, appErr.Code, appErr.Message)
			}
			return NewErrorEvent(operationID, machineID, errors.ErrImagePullFailed.Code, errors.ErrImagePullFailed.Message)
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
		imageID := chi.URLParam(r, "imageId")
		if imageID == "" {
			return struct{}{}, errors.NewAppError("MISSING_IMAGE_ID", "Image ID is required", http.StatusBadRequest)
		}

		force := false
		if forceParam := r.URL.Query().Get("force"); forceParam != "" {
			if parsed, err := strconv.ParseBool(forceParam); err == nil {
				force = parsed
			}
		}

		return struct{}{}, h.service.Delete(ctx, userID, machineID, imageID, force)
	})
}
