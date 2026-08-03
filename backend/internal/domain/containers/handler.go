package containers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/streaming"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// ContainerHandler handles container requests
type ContainerHandler struct {
	service        ContainerServiceInterface
	machineService *any
}

// NewContainerHandler creates a new ContainerHandler
func NewContainerHandler(service ContainerServiceInterface, machineService any) *ContainerHandler {
	return &ContainerHandler{
		service:        service,
		machineService: &machineService,
	}
}

// List handles listing containers
func (h *ContainerHandler) List(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "List")
	// Get user ID from context
	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), errors.ErrInvalidCredentials.Code)
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

	slog.InfoContext(r.Context(), "list", slog.Any("opts", opts))

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
		slog.ErrorContext(r.Context(), "list", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID))
		return
	}

	// Convert podman entities to API DTO response
	containersAny := make([]any, len(containers))
	for i, c := range containers {
		containersAny[i] = c
	}
	result := ToContainerListResult(containersAny)

	slog.InfoContext(r.Context(), "list", slog.Any("result", result))

	// Return success response with container list DTO
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// Create handles creating a container
func (h *ContainerHandler) Create(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Create")

	// Get user ID from context
	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), errors.ErrInvalidCredentials.Code)
		return
	}

	// Get machine ID from URL parameter
	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Parse request body
	var request ContainerCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "Invalid request body", slog.Any("error", err))
		return
	}

	// Validate request
	validate := validator.New()
	if err := validate.Struct(request); err != nil {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "Validation failed", slog.Any("error", err))
		return
	}

	slog.InfoContext(r.Context(), "create", slog.Any("request", request))

	// Call ContainerService.Create
	response, err := h.service.Create(r.Context(), userID, machineID, request)
	if err != nil {
		// Handle specific error types
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		// Handle generic errors
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "create", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID))
		return
	}

	slog.InfoContext(r.Context(), "create", slog.Any("response", response))

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// Get handles getting a container by ID
func (h *ContainerHandler) Get(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Get")

	// Get user ID from context
	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), errors.ErrInvalidCredentials.Code)
		return
	}

	// Get machine ID from URL parameter
	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Get container ID from URL parameter
	containerID := chi.URLParam(r, "containerId")
	if containerID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_CONTAINER_ID", "Container ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	slog.InfoContext(r.Context(), "get", slog.String("containerID", containerID))

	// Call ContainerService.Get with context, userID, machineID, and containerID
	containerData, err := h.service.Get(r.Context(), userID, machineID, containerID)
	if err != nil {
		// Handle specific error types
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		// Handle generic errors
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "get", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID), slog.Any("containerID", containerID))
		return
	}

	// Convert podman entities to API DTO response
	result := FromPodmanInspectContainer(containerData)

	slog.InfoContext(r.Context(), "get", slog.Any("result", result))

	// Return success response with container details DTO
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// Start handles starting a container
func (h *ContainerHandler) Start(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Start")

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

	containerID := chi.URLParam(r, "containerId")
	if containerID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_CONTAINER_ID", "Container ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	slog.InfoContext(r.Context(), "start", slog.String("containerID", containerID))

	response, err := h.service.Start(r.Context(), userID, machineID, containerID)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "start", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID), slog.Any("containerID", containerID))
		return
	}

	slog.InfoContext(r.Context(), "start", slog.Any("response", response))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Stop handles stopping a container
func (h *ContainerHandler) Stop(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Stop")

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

	containerID := chi.URLParam(r, "containerId")
	if containerID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_CONTAINER_ID", "Container ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	slog.InfoContext(r.Context(), "stop", slog.String("containerID", containerID))

	response, err := h.service.Stop(r.Context(), userID, machineID, containerID)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "stop", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID), slog.Any("containerID", containerID))
		return
	}

	slog.InfoContext(r.Context(), "stop", slog.Any("response", response))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Delete handles deleting a container
func (h *ContainerHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	containerID := chi.URLParam(r, "containerId")
	if containerID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_CONTAINER_ID", "Container ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	force := false
	if forceParam := r.URL.Query().Get("force"); forceParam != "" {
		if parsed, err := strconv.ParseBool(forceParam); err == nil {
			force = parsed
		}
	}

	slog.InfoContext(r.Context(), "delete", slog.String("containerID", containerID), slog.Bool("force", force))

	err := h.service.Delete(r.Context(), userID, machineID, containerID, force)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "delete", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID), slog.Any("containerID", containerID))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Logs handles retrieving logs for a container
func (h *ContainerHandler) Logs(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Logs")

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

	containerID := chi.URLParam(r, "containerId")
	if containerID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_CONTAINER_ID", "Container ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Parse query parameters
	var since time.Time
	var tail int64 = -1

	// Parse 'since' parameter (ISO8601 timestamp)
	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		parsedSince, err := time.Parse(time.RFC3339, sinceParam)
		if err != nil {
			errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_SINCE_PARAM", "Invalid 'since' parameter format, use ISO8601", contextutils.GetRequestID(r.Context()))
			slog.ErrorContext(r.Context(), "Invalid since parameter", slog.Any("error", err))
			return
		}
		since = parsedSince
	}

	// Parse 'tail' parameter (number of lines)
	if tailParam := r.URL.Query().Get("tail"); tailParam != "" {
		parsedTail, err := strconv.ParseInt(tailParam, 10, 64)
		if err != nil {
			errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_TAIL_PARAM", "Invalid 'tail' parameter, must be an integer", contextutils.GetRequestID(r.Context()))
			slog.ErrorContext(r.Context(), "Invalid tail parameter", slog.Any("error", err))
			return
		}
		tail = parsedTail
	}

	// Parse 'follow' parameter (streaming mode)
	follow := false
	if followParam := r.URL.Query().Get("follow"); followParam != "" {
		if parsed, err := strconv.ParseBool(followParam); err == nil {
			follow = parsed
		}
	}

	slog.InfoContext(r.Context(), "logs", slog.String("containerID", containerID), slog.Time("since", since), slog.Int64("tail", tail), slog.Bool("follow", follow))

	// Streaming mode when follow=true
	if follow {
		h.streamLogs(w, r, userID, machineID, containerID, since, tail)
		return
	}

	response, err := h.service.GetLogs(r.Context(), userID, machineID, containerID, since, tail)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "logs", slog.Any("error", err), slog.Any("userID", userID), slog.Any("machineID", machineID), slog.Any("containerID", containerID))
		return
	}

	slog.InfoContext(r.Context(), "logs", slog.String("containerID", containerID), slog.Time("since", since), slog.Int64("tail", tail), slog.String("responseID", response.ID))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// streamLogs handles streaming logs for a container
func (h *ContainerHandler) streamLogs(w http.ResponseWriter, r *http.Request, userID, machineID, containerID string, since time.Time, tail int64) {
	slog.InfoContext(r.Context(), "streamLogs", slog.String("containerID", containerID), slog.Bool("follow", true))

	// Create a cancelable context for the streaming operation
	streamCtx, cancelStream := context.WithCancel(r.Context())
	defer cancelStream()

	// Set headers for streaming NDJSON response
	streaming.SetNDJSONHeaders(w)

	// Create NDJSON writer
	ndjsonWriter, err := streaming.NewNDJSONWriter(w)
	if err != nil {
		errors.RespondWithHTTPError(w, http.StatusInternalServerError, "STREAMING_NOT_SUPPORTED", "Streaming not supported", contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "Streaming not supported")
		return
	}

	// Create a channel to detect client disconnect
	disconnectChan := streaming.DisconnectChan(streamCtx)

	// Create a channel for log events
	logEventChan := make(chan ContainerLogEvent, 10)
	errorChan := make(chan error, 1)

	// Start streaming logs in a goroutine
	go func() {
		err := h.service.GetLogsWithStream(streamCtx, userID, machineID, containerID, since, tail, func(event ContainerLogEvent) {
			select {
			case logEventChan <- event:
			case <-streamCtx.Done():
			}
		})
		if err != nil {
			errorChan <- err
		}
	}()

	// Stream events to client
	streamComplete := false
	for !streamComplete {
		select {
		case event := <-logEventChan:
			if err := ndjsonWriter.Write(event); err != nil {
				slog.ErrorContext(r.Context(), "Failed to write log event", slog.Any("error", err))
				cancelStream()
				streamComplete = true
			}

		case err := <-errorChan:
			// Streaming failed or completed
			var errorEvent ContainerLogEvent
			if streamCtx.Err() == context.Canceled {
				errorEvent = ContainerLogEvent{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Level:     "WARN",
					Message:   "OPERATION_CANCELLED",
				}
			} else if appErr, ok := err.(*errors.AppError); ok {
				if appErr == errors.ErrContainerNotFound {
					errorEvent = ContainerLogEvent{
						Timestamp: time.Now().UTC().Format(time.RFC3339),
						Level:     "ERROR",
						Message:   "CONTAINER_NOT_FOUND: " + appErr.Message,
					}
				} else if appErr == errors.ErrOperationCancelled {
					errorEvent = ContainerLogEvent{
						Timestamp: time.Now().UTC().Format(time.RFC3339),
						Level:     "WARN",
						Message:   "OPERATION_CANCELLED: " + appErr.Message,
					}
				} else {
					errorEvent = ContainerLogEvent{
						Timestamp: time.Now().UTC().Format(time.RFC3339),
						Level:     "ERROR",
						Message:   appErr.Code + ": " + appErr.Message,
					}
				}
			} else {
				errorEvent = ContainerLogEvent{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Level:     "ERROR",
					Message:   "CONNECTION_FAILED: " + err.Error(),
				}
			}
			ndjsonWriter.Write(errorEvent)
			streamComplete = true

		case <-disconnectChan:
			// Client disconnected
			slog.InfoContext(r.Context(), "Client disconnected, cancelling log stream")
			cancelStream()
			// Try to send cancellation event (best effort)
			errorEvent := ContainerLogEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     "WARN",
				Message:   "OPERATION_CANCELLED",
			}
			ndjsonWriter.Write(errorEvent)
			streamComplete = true
		}
	}
}
