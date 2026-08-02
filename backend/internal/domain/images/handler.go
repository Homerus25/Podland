package images

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/errors"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// ImageHandler handles image requests
type ImageHandler struct {
	service        ImageServiceInterface
	machineService any
}

// NewImageHandler creates a new ImageHandler
func NewImageHandler(service ImageServiceInterface, machineService any) *ImageHandler {
	return &ImageHandler{
		service:        service,
		machineService: machineService,
	}
}

// List handles listing images
func (h *ImageHandler) List(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "List")

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

	opts := entities.ImageListOptions{}

	if allParam := r.URL.Query().Get("all"); allParam != "" {
		all, err := strconv.ParseBool(allParam)
		if err == nil {
			opts.All = all
		}
	}

	if filtersParam := r.URL.Query().Get("filters"); filtersParam != "" {
		var filters map[string][]string
		if err := json.Unmarshal([]byte(filtersParam), &filters); err != nil {
			slog.WarnContext(r.Context(), "invalid filters query param, ignoring", slog.String("filters", filtersParam), slog.Any("error", err))
		} else {
			for key, values := range filters {
				for _, value := range values {
					opts.Filter = append(opts.Filter, key+"="+value)
				}
			}
		}
	}

	slog.InfoContext(r.Context(), "list", slog.Any("opts", opts))

	images, err := h.service.List(r.Context(), userID, machineID, opts)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "list", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID))
		return
	}

	result := ToImageListResponse(images)

	slog.InfoContext(r.Context(), "list", slog.Any("result", result))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// Pull handles pulling an image
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

	report, err := h.service.Pull(r.Context(), userID, machineID, request.Image, pullOpts)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "pull", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID))
		return
	}

	result := FromImagePullReport(report)

	slog.InfoContext(r.Context(), "pull", slog.Any("result", result))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// Delete handles deleting an image
func (h *ImageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Delete")

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

	imageID := chi.URLParam(r, "imageId")
	if imageID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_IMAGE_ID", "Image ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	force := false
	if forceParam := r.URL.Query().Get("force"); forceParam != "" {
		if parsed, err := strconv.ParseBool(forceParam); err == nil {
			force = parsed
		}
	}

	slog.InfoContext(r.Context(), "delete", slog.String("imageID", imageID), slog.Bool("force", force))

	err := h.service.Delete(r.Context(), userID, machineID, imageID, force)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "delete", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID), slog.Any("imageID", imageID))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
