package quadlets

import (
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
	slog.InfoContext(r.Context(), "Update quadlet")

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

	var request QuadletUpdateRequest
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

	quadlet, err := h.service.Update(r.Context(), userID, machineID, quadletID, request)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "update", slog.Any("error", err), slog.String("userID", userID), slog.String("machineID", machineID), slog.String("quadletId", quadletID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(quadlet)
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

// Start handles starting a quadlet-managed resource.
// Runs `systemctl start <unit>` over SSH via the service.
// Returns 200 with {id, state, unit}, or QUADLET_ALREADY_RUNNING (400) /
// QUADLET_NOT_FOUND (404) / CONNECTION_FAILED (502) on error.
func parseLifecyclePath(r *http.Request) (userID, machineID, quadletID string, err error) {
	userID = contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		return "", "", "", errors.ErrInvalidCredentials
	}

	machineID = chi.URLParam(r, "id")
	if machineID == "" {
		return "", "", "", errors.NewAppError("MISSING_MACHINE_ID", "Machine ID is required", http.StatusBadRequest)
	}

	quadletID = chi.URLParam(r, "quadletId")
	if quadletID == "" {
		return "", "", "", errors.NewAppError("MISSING_QUADLET_ID", "Quadlet ID is required", http.StatusBadRequest)
	}

	return userID, machineID, quadletID, nil
}

func (h *QuadletHandler) Start(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Start quadlet",
		slog.String("quadletId", chi.URLParam(r, "quadletId")))

	userID, machineID, quadletID, err := parseLifecyclePath(r)
	if err != nil {
		if err == errors.ErrInvalidCredentials {
			slog.ErrorContext(r.Context(), errors.ErrInvalidCredentials.Code)
		}
		errors.RespondWithError(w, err.(*errors.AppError), contextutils.GetRequestID(r.Context()))
		return
	}

	resp, err := h.service.Start(r.Context(), userID, machineID, quadletID)
	if err != nil {
		respondLifecycleError(w, r, err, "start", userID, machineID, quadletID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// Stop handles stopping a running quadlet-managed resource.
// Runs `systemctl stop <unit>` over SSH, honouring the `force` and `timeout`
// query params (doku/description_api.md:1019-1022).
func (h *QuadletHandler) Stop(w http.ResponseWriter, r *http.Request) {
	userID, machineID, quadletID, err := parseLifecyclePath(r)
	if err != nil {
		if err == errors.ErrInvalidCredentials {
			slog.ErrorContext(r.Context(), errors.ErrInvalidCredentials.Code)
		}
		errors.RespondWithError(w, err.(*errors.AppError), contextutils.GetRequestID(r.Context()))
		return
	}

	opts := parseLifecycleOptions(r)

	slog.InfoContext(r.Context(), "stop",
		slog.String("quadletId", quadletID),
		slog.Bool("force", opts.Force),
		slog.Int("timeout", opts.Timeout),
	)

	resp, err := h.service.Stop(r.Context(), userID, machineID, quadletID, opts)
	if err != nil {
		respondLifecycleError(w, r, err, "stop", userID, machineID, quadletID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// Restart handles restarting a quadlet-managed resource.
// Runs `systemctl restart <unit>` over SSH, honouring the `force` and `timeout`
// query params (doku/description_api.md:1048-1050).
func (h *QuadletHandler) Restart(w http.ResponseWriter, r *http.Request) {
	userID, machineID, quadletID, err := parseLifecyclePath(r)
	if err != nil {
		if err == errors.ErrInvalidCredentials {
			slog.ErrorContext(r.Context(), errors.ErrInvalidCredentials.Code)
		}
		errors.RespondWithError(w, err.(*errors.AppError), contextutils.GetRequestID(r.Context()))
		return
	}

	opts := parseLifecycleOptions(r)

	slog.InfoContext(r.Context(), "restart",
		slog.String("quadletId", quadletID),
		slog.Bool("force", opts.Force),
		slog.Int("timeout", opts.Timeout),
	)

	resp, err := h.service.Restart(r.Context(), userID, machineID, quadletID, opts)
	if err != nil {
		respondLifecycleError(w, r, err, "restart", userID, machineID, quadletID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// parseLifecycleOptions reads the `force` (bool) and `timeout` (int, seconds)
// query parameters for stop/restart. force defaults to false; an unparseable
// value is ignored (stays false), matching the Delete handler convention.
// timeout defaults to 0 (unset); an unparseable or negative value is ignored.
func parseLifecycleOptions(r *http.Request) LifecycleOptions {
	opts := LifecycleOptions{}
	if forceParam := r.URL.Query().Get("force"); forceParam != "" {
		if parsed, err := strconv.ParseBool(forceParam); err == nil {
			opts.Force = parsed
		}
	}
	if timeoutParam := r.URL.Query().Get("timeout"); timeoutParam != "" {
		if parsed, err := strconv.Atoi(timeoutParam); err == nil && parsed > 0 {
			opts.Timeout = parsed
		}
	}
	return opts
}

// respondLifecycleError maps a service error to the appropriate HTTP response
// for the lifecycle endpoints, mirroring the error-handling shape used by the
// other quadlet handlers. action is used for log attribution only.
func respondLifecycleError(w http.ResponseWriter, r *http.Request, err error, action, userID, machineID, quadletID string) {
	if appErr, ok := err.(*errors.AppError); ok {
		errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
		return
	}
	errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
	slog.ErrorContext(r.Context(), action,
		slog.Any("error", err),
		slog.String("userID", userID),
		slog.String("machineID", machineID),
		slog.String("quadletId", quadletID),
	)
}

// GetLogs handles getting quadlet logs
func (h *QuadletHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Get quadlet logs")

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

	// Parse query parameters
	// follow defaults to false
	follow := false
	if followParam := r.URL.Query().Get("follow"); followParam != "" {
		if parsed, err := strconv.ParseBool(followParam); err == nil {
			follow = parsed
		}
	}

	// Parse since parameter (ISO8601 timestamp)
	since := time.Time{}
	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		parsedSince, err := time.Parse(time.RFC3339, sinceParam)
		if err != nil {
			errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_SINCE_PARAM", "Invalid since parameter format (use RFC3339)", contextutils.GetRequestID(r.Context()))
			slog.ErrorContext(r.Context(), "Invalid since parameter", slog.Any("error", err), slog.String("since", sinceParam))
			return
		}
		since = parsedSince
	}

	// Parse tail parameter (number of lines, -1 for all)
	tail := int64(-1)
	if tailParam := r.URL.Query().Get("tail"); tailParam != "" {
		if parsed, err := strconv.ParseInt(tailParam, 10, 64); err == nil {
			tail = parsed
		}
	}

	// Non-streaming mode
	if !follow {
		logs, err := h.service.GetLogs(r.Context(), userID, machineID, quadletID, since, tail)
		if err != nil {
			if appErr, ok := err.(*errors.AppError); ok {
				errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
				return
			}
			errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
			slog.ErrorContext(r.Context(), "get logs", slog.Any("error", err), slog.String("userID", userID), slog.String("machineID", machineID), slog.String("quadletId", quadletID))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(logs)
		return
	}

	// Streaming mode (follow=true)
	streaming.SetNDJSONHeaders(w)
	w.WriteHeader(http.StatusOK)

	ndjsonWriter, err := streaming.NewNDJSONWriter(w)
	if err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "Failed to create NDJSON writer", slog.Any("error", err))
		return
	}

	// Create a callback writer that converts QuadletLogEvent to NDJSON
	logCallback := func(event QuadletLogEvent) {
		if err := ndjsonWriter.Write(event); err != nil {
			slog.ErrorContext(r.Context(), "Failed to write log event", slog.Any("error", err))
			return
		}
	}

	// Create a disconnect channel
	disconnectChan := streaming.DisconnectChan(r.Context())

	// Start streaming logs in a goroutine
	doneChan := make(chan error, 1)
	go func() {
		doneChan <- h.service.GetLogsWithStream(r.Context(), userID, machineID, quadletID, since, tail, logCallback)
	}()

	// Wait for either completion or client disconnect
	select {
	case err := <-doneChan:
		if err != nil {
			// Emit error event to the stream
			errorEvent := map[string]interface{}{
				"level":   "ERROR",
				"message": err.Error(),
			}
			if appErr, ok := err.(*errors.AppError); ok {
				errorEvent["code"] = appErr.Code
				errorEvent["message"] = appErr.Message
			}
			ndjsonWriter.Write(errorEvent)
		}
	case <-disconnectChan:
		// Client disconnected, emit cancellation event
		ndjsonWriter.Write(map[string]interface{}{
			"level":   "ERROR",
			"code":    "OPERATION_CANCELLED",
			"message": "Client disconnected",
		})
		// Wait for streaming to complete and avoid goroutine leak
		go func() {
			<-doneChan
		}()
	}
}

// Generate handles generating a quadlet
func (h *QuadletHandler) Generate(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Import handles importing quadlets
func (h *QuadletHandler) Import(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
