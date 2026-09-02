package volumes

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
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

// Create handles creating a volume on a machine. Route-level
// RequireMachinePermission(write) (app.go) gates it at write. The body is
// optional in shape: an absent/empty name creates an anonymous volume.
// Name-format validation is podman's job; its rejections surface as 400
// INVALID_REQUEST from the service.
func (h *VolumeHandler) Create(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Create")

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

	var request VolumeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "Invalid volume create request body")
		return
	}

	report, err := h.service.Create(r.Context(), userID, machineID, request)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "create", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID))
		return
	}

	result := FromPodmanVolumeInspect(report)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// Delete handles removing a volume by exact name. Route-level
// RequireMachinePermission(write) (app.go) gates it at write. The force
// query flag (default false) lets podman stop dependent containers and
// remove the volume anyway; without it an in-use volume surfaces as 409
// VOLUME_IN_USE. Returns 204 with an empty body.
func (h *VolumeHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	volumeName := chi.URLParam(r, "volumeName")
	if volumeName == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_VOLUME_NAME", "Volume name is required", contextutils.GetRequestID(r.Context()))
		return
	}

	force := false
	if raw := r.URL.Query().Get("force"); raw != "" {
		var err error
		force, err = strconv.ParseBool(raw)
		if err != nil {
			errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_REQUEST", "force must be a boolean", contextutils.GetRequestID(r.Context()))
			return
		}
	}

	if err := h.service.Delete(r.Context(), userID, machineID, volumeName, force); err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "delete", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID), slog.Any("volumeName", volumeName))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Prune handles removing all unused volumes on a machine. Route-level
// RequireMachinePermission(write) (app.go) gates it at write; podman
// leaves in-use volumes alone. Returns 200 with one report entry per
// removed volume (name, reclaimed size, any per-volume error).
func (h *VolumeHandler) Prune(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Prune")

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

	prunes, err := h.service.Prune(r.Context(), userID, machineID)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "prune", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID))
		return
	}

	result := FromPodmanPruneReports(prunes)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// Export handles streaming a volume's contents as a tar download.
// Read-level permission suffices (subtree RequireMachineAccess): users
// who can read a machine's state can read its data. Headers are set
// before the service streams, so the download starts with the right
// Content-Type/Disposition; a 404/4xx happens before any byte is
// written (the service resolves existence first), so error envelopes
// stay intact. Mid-stream failures can't retroactively change the
// status — the connection just truncates.
func (h *VolumeHandler) Export(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Export")

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

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", volumeName+".tar"))

	if err := h.service.Export(r.Context(), userID, machineID, volumeName, w); err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "export", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID), slog.Any("volumeName", volumeName))
		return
	}
}
