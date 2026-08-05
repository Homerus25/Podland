package quadlets

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/errors"
)

// QuadletHandler handles quadlet requests
type QuadletHandler struct {
	service        QuadletServiceInterface
	machineService any
}

// NewQuadletHandler creates a new QuadletHandler
func NewQuadletHandler(service QuadletServiceInterface, machineService any) *QuadletHandler {
	return &QuadletHandler{
		service:        service,
		machineService: machineService,
	}
}

// List handles listing quadlets
func (h *QuadletHandler) List(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "List quadlets")

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

	filters := ListFilters{
		Resource: r.URL.Query().Get("resource"),
		Name:     r.URL.Query().Get("name"),
	}

	quadlets, err := h.service.List(r.Context(), userID, machineID, filters)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "list", slog.Any("error", err), slog.String("userID", userID), slog.String("machineID", machineID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(quadlets)
}

// Create handles creating a quadlet
func (h *QuadletHandler) Create(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Create quadlet")

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

	var request QuadletCreateRequest
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

	quadlet, err := h.service.Create(r.Context(), userID, machineID, request)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "create", slog.Any("error", err), slog.String("userID", userID), slog.String("machineID", machineID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(quadlet)
}

// Get handles getting a quadlet by ID
func (h *QuadletHandler) Get(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Get quadlet")

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

	quadletID := chi.URLParam(r, "quadletId")
	if quadletID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_QUADLET_ID", "Quadlet ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	quadlet, err := h.service.Get(r.Context(), userID, machineID, quadletID)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "get", slog.Any("error", err), slog.String("userID", userID), slog.String("machineID", machineID), slog.String("quadletId", quadletID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(quadlet)
}

// Update handles updating a quadlet
func (h *QuadletHandler) Update(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Delete handles deleting a quadlet
func (h *QuadletHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Delete quadlet")

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

	quadletID := chi.URLParam(r, "quadletId")
	if quadletID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_QUADLET_ID", "Quadlet ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// force defaults to false; an unparseable value is ignored (stays false),
	// matching the images handler convention.
	force := false
	if forceParam := r.URL.Query().Get("force"); forceParam != "" {
		if parsed, err := strconv.ParseBool(forceParam); err == nil {
			force = parsed
		}
	}

	// stop defaults to true per the contract (doku/description_api.md:972).
	// Only an explicit "false" (or other strconv-false value) turns it off;
	// an unparseable value leaves the default in place.
	stop := true
	if stopParam := r.URL.Query().Get("stop"); stopParam != "" {
		if parsed, err := strconv.ParseBool(stopParam); err == nil {
			stop = parsed
		}
	}

	slog.InfoContext(r.Context(), "delete",
		slog.String("quadletId", quadletID),
		slog.Bool("force", force),
		slog.Bool("stop", stop),
	)

	err := h.service.Delete(r.Context(), userID, machineID, quadletID, force, stop)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "delete",
			slog.Any("error", err),
			slog.String("userID", userID),
			slog.String("machineID", machineID),
			slog.String("quadletId", quadletID),
		)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Start handles starting a quadlet
func (h *QuadletHandler) Start(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Stop handles stopping a quadlet
func (h *QuadletHandler) Stop(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Restart handles restarting a quadlet
func (h *QuadletHandler) Restart(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// GetLogs handles getting quadlet logs
func (h *QuadletHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Generate handles generating a quadlet
func (h *QuadletHandler) Generate(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Import handles importing quadlets
func (h *QuadletHandler) Import(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
