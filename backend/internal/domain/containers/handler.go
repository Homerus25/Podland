package containers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/errors"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// ContainerHandler handles container requests
type ContainerHandler struct {
	service        *ContainerService
	machineService *any
}

// NewContainerHandler creates a new ContainerHandler
func NewContainerHandler(service *ContainerService, machineService any) *ContainerHandler {
	return &ContainerHandler{
		service:        service,
		machineService: &machineService,
	}
}

// List handles listing containers
func (h *ContainerHandler) List(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine ID from URL parameter
	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Parse query parameters for container list options
	opts := entities.ContainerListOptions{}

	// Parse 'all' parameter
	if allParam := r.URL.Query().Get("all"); allParam != "" {
		all, err := strconv.ParseBool(allParam)
		if err == nil {
			opts.All = all
		}
	}

	// Parse 'filters' parameter (comma-separated key=value pairs)
	if filtersParam := r.URL.Query().Get("filters"); filtersParam != "" {
		opts.Filters = make(map[string][]string)
		// Simple parsing - split by comma and then by equals
		// This is a basic implementation; could be enhanced
		// Expected format: key=value,key2=value2
		// For now, we'll just store as-is
		opts.Filters["custom"] = []string{filtersParam}
	}

	// Parse 'size' parameter
	if sizeParam := r.URL.Query().Get("size"); sizeParam != "" {
		size, err := strconv.ParseBool(sizeParam)
		if err == nil {
			opts.Size = size
		}
	}

	// Parse 'namespace' parameter
	if namespaceParam := r.URL.Query().Get("namespace"); namespaceParam != "" {
		namespace, err := strconv.ParseBool(namespaceParam)
		if err == nil {
			opts.Namespace = namespace
		}
	}

	// Parse 'sync' parameter
	if syncParam := r.URL.Query().Get("sync"); syncParam != "" {
		sync, err := strconv.ParseBool(syncParam)
		if err == nil {
			opts.Sync = sync
		}
	}

	// Parse 'external' parameter
	if externalParam := r.URL.Query().Get("external"); externalParam != "" {
		external, err := strconv.ParseBool(externalParam)
		if err == nil {
			opts.External = external
		}
	}

	// Parse 'last' parameter
	if lastParam := r.URL.Query().Get("last"); lastParam != "" {
		last, err := strconv.Atoi(lastParam)
		if err == nil {
			opts.Last = last
		}
	}

	// Parse 'latest' parameter
	if latestParam := r.URL.Query().Get("latest"); latestParam != "" {
		latest, err := strconv.ParseBool(latestParam)
		if err == nil {
			opts.Latest = latest
		}
	}

	// Call ContainerService.List with context, userID, machineID, and options
	containers, err := h.service.List(r.Context(), userID, machineID, opts)
	if err != nil {
		// Handle specific error types
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		// Handle generic errors
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Return success response with container list
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(containers)
}

// Create handles creating a container
func (h *ContainerHandler) Create(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Get handles getting a container by ID
func (h *ContainerHandler) Get(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Start handles starting a container
func (h *ContainerHandler) Start(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Stop handles stopping a container
func (h *ContainerHandler) Stop(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Delete handles deleting a container
func (h *ContainerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
