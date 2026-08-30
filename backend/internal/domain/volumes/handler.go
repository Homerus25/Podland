package volumes

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// VolumeHandler handles volume requests
type VolumeHandler struct {
	service VolumeServiceInterface
}

// NewVolumeHandler creates a new VolumeHandler
func NewVolumeHandler(service VolumeServiceInterface) *VolumeHandler {
	return &VolumeHandler{
		service: service,
	}
}

// List handles listing volumes on a machine
func (h *VolumeHandler) List(w http.ResponseWriter, r *http.Request) {
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

	opts := entities.VolumeListOptions{}

	volumes, err := h.service.List(r.Context(), userID, machineID, opts)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "list", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID))
		return
	}

	result := ToVolumeListResponse(volumes)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// Get handles inspecting a single volume on a machine
func (h *VolumeHandler) Get(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Get")

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

	volumeName := chi.URLParam(r, "volumeName")
	if volumeName == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_VOLUME_NAME", "Volume name is required", contextutils.GetRequestID(r.Context()))
		return
	}

	report, err := h.service.Get(r.Context(), userID, machineID, volumeName)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "get", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID), slog.Any("volumeName", volumeName))
		return
	}

	result := FromPodmanVolumeInspect(report)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}
