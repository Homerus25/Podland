package quadlets

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

// QuadletHandler handles quadlet requests
//
// JSON endpoints cross the endpoint envelope (internal/web). GetLogs
// and AutoUpdate keep their own shape — they stream NDJSON rather than
// encoding one DTO.
type QuadletHandler struct {
	service QuadletServiceInterface
}

// NewQuadletHandler creates a new QuadletHandler
func NewQuadletHandler(service QuadletServiceInterface) *QuadletHandler {
	return &QuadletHandler{
		service: service,
	}
}

// missingQuadletID is the guard the quadlet-scoped endpoints share:
// the {quadletId} URL param is required.
func missingQuadletID(r *http.Request) error {
	if chi.URLParam(r, "quadletId") == "" {
		return errors.NewAppError("MISSING_QUADLET_ID", "Quadlet ID is required", http.StatusBadRequest)
	}
	return nil
}

// List handles listing quadlets
func (h *QuadletHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]QuadletListResponse, error) {
		filters := ListFilters{
			Resource: r.URL.Query().Get("resource"),
			Name:     r.URL.Query().Get("name"),
		}
		return h.service.List(ctx, userID, machineID, filters)
	})
}

// Create handles creating a quadlet
func (h *QuadletHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (*QuadletCreateResponse, error) {
		var request QuadletCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			return nil, errors.ErrInvalidRequest.WithMessage("Invalid request body")
		}

		validate := validator.New()
		if err := validate.Struct(request); err != nil {
			return nil, errors.NewAppError("VALIDATION_FAILED", err.Error(), http.StatusBadRequest)
		}

		return h.service.Create(ctx, userID, machineID, request)
	})
}

// Get handles getting a quadlet by ID
func (h *QuadletHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletGetResponse, error) {
		if err := missingQuadletID(r); err != nil {
			return nil, err
		}
		return h.service.Get(ctx, userID, machineID, chi.URLParam(r, "quadletId"))
	})
}

// Update handles updating a quadlet
func (h *QuadletHandler) Update(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletGetResponse, error) {
		if err := missingQuadletID(r); err != nil {
			return nil, err
		}

		var request QuadletUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			return nil, errors.ErrInvalidRequest.WithMessage("Invalid request body")
		}

		validate := validator.New()
		if err := validate.Struct(request); err != nil {
			return nil, errors.NewAppError("VALIDATION_FAILED", err.Error(), http.StatusBadRequest)
		}

		return h.service.Update(ctx, userID, machineID, chi.URLParam(r, "quadletId"), request)
	})
}

// Delete handles deleting a quadlet. force defaults to false and stop
// defaults to true (doku/description_quadlets_api.md); unparseable
// values leave the defaults in place.
func (h *QuadletHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, machineID string) (struct{}, error) {
		if err := missingQuadletID(r); err != nil {
			return struct{}{}, err
		}
		quadletID := chi.URLParam(r, "quadletId")

		force := false
		if forceParam := r.URL.Query().Get("force"); forceParam != "" {
			if parsed, err := strconv.ParseBool(forceParam); err == nil {
				force = parsed
			}
		}

		stop := true
		if stopParam := r.URL.Query().Get("stop"); stopParam != "" {
			if parsed, err := strconv.ParseBool(stopParam); err == nil {
				stop = parsed
			}
		}

		return struct{}{}, h.service.Delete(ctx, userID, machineID, quadletID, force, stop)
	})
}

// Start handles starting a quadlet-managed resource.
// Runs `systemctl start <unit>` over SSH via the service.
// Returns 200 with {id, state, unit}, or QUADLET_ALREADY_RUNNING (400) /
// QUADLET_NOT_FOUND (404) / CONNECTION_FAILED (502) on error.
func (h *QuadletHandler) Start(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletLifecycleResponse, error) {
		if err := missingQuadletID(r); err != nil {
			return nil, err
		}
		return h.service.Start(ctx, userID, machineID, chi.URLParam(r, "quadletId"))
	})
}

// Stop handles stopping a running quadlet-managed resource.
// Runs `systemctl stop <unit>` over SSH, honouring the `force` and `timeout`
// query params (doku/description_quadlets_api.md:194-196).
func (h *QuadletHandler) Stop(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletLifecycleResponse, error) {
		if err := missingQuadletID(r); err != nil {
			return nil, err
		}
		return h.service.Stop(ctx, userID, machineID, chi.URLParam(r, "quadletId"), parseLifecycleOptions(r))
	})
}

// Restart handles restarting a quadlet-managed resource.
// Runs `systemctl restart <unit>` over SSH, honouring the `force` and `timeout`
// query params (doku/description_quadlets_api.md:223-225).
func (h *QuadletHandler) Restart(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletLifecycleResponse, error) {
		if err := missingQuadletID(r); err != nil {
			return nil, err
		}
		return h.service.Restart(ctx, userID, machineID, chi.URLParam(r, "quadletId"), parseLifecycleOptions(r))
	})
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

// logStreamShutdownTimeout bounds how long the follow-mode logs handler
// waits for the streaming goroutine to notice the canceled request context
// after a client disconnect. It only trips on a stream that ignores
// cancellation; healthy streams stop well within it.
const logStreamShutdownTimeout = 15 * time.Second

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
	// Track write errors to stop streaming when client disconnects
	var writeError atomic.Bool
	logCallback := func(event QuadletLogEvent) {
		if writeError.Load() {
			return
		}
		if err := ndjsonWriter.Write(event); err != nil {
			slog.ErrorContext(r.Context(), "Failed to write log event", slog.Any("error", err))
			writeError.Store(true)
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
			errorEvent := map[string]any{
				"timestamp": time.Now().UTC().Format(time.RFC3339),
				"level":     "ERROR",
				"message":   err.Error(),
			}
			if appErr, ok := err.(*errors.AppError); ok {
				errorEvent["code"] = appErr.Code
				errorEvent["message"] = appErr.Message
			}
			ndjsonWriter.Write(errorEvent)
		}
	case <-disconnectChan:
		// Client disconnected, emit cancellation event
		ndjsonWriter.Write(map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"level":     "ERROR",
			"code":      "OPERATION_CANCELLED",
			"message":   "Client disconnected",
		})
		// Stop accepting further events: logCallback checks this before
		// every write, so the streaming goroutine becomes a no-op even
		// before it notices the canceled context.
		writeError.Store(true)
		// Wait for GetLogsWithStream to return before this handler
		// returns. The callback writes to this handler's ResponseWriter,
		// and a write after the handler returned lands in the connection's
		// recycled write buffer — corrupting whatever response is served
		// on the reused keep-alive connection next. ContainerLogs aborts
		// on the canceled request context, so this wait is brief; the
		// timeout only guards a pathological stream that ignores
		// cancellation (in which case the warning surfaces it).
		select {
		case <-doneChan:
		case <-time.After(logStreamShutdownTimeout):
			slog.WarnContext(r.Context(), "log stream did not stop after client disconnect",
				slog.String("machineID", machineID), slog.String("quadletId", quadletID))
		}
	}
}

// Generate handles generating a quadlet from an existing container or pod
func (h *QuadletHandler) Generate(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletGenerateResponse, error) {
		var request QuadletGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			return nil, errors.ErrInvalidRequest.WithMessage("Invalid request body")
		}

		validate := validator.New()
		if err := validate.Struct(request); err != nil {
			return nil, errors.NewAppError("VALIDATION_FAILED", err.Error(), http.StatusBadRequest)
		}

		return h.service.Generate(ctx, userID, machineID, request)
	})
}

// Import handles importing quadlets from uploaded content
func (h *QuadletHandler) Import(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (*QuadletImportResponse, error) {
		var request QuadletImportRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			return nil, errors.ErrInvalidRequest.WithMessage("Invalid request body")
		}

		validate := validator.New()
		if err := validate.Struct(request); err != nil {
			return nil, errors.NewAppError("VALIDATION_FAILED", err.Error(), http.StatusBadRequest)
		}

		return h.service.Import(ctx, userID, machineID, request)
	})
}

// AutoUpdate handles running `podman auto-update` on the machine with live
// NDJSON streaming of remote output, terminating in a parsed report event.
// Registry pulls make runs routinely exceed the global 30-second write
// timeout, so this handler extends its own per-response write deadline to
// 10 minutes without touching the server-wide setting.
func (h *QuadletHandler) AutoUpdate(w http.ResponseWriter, r *http.Request) {
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

	dryRun, _ := strconv.ParseBool(r.URL.Query().Get("dryRun"))

	operationID := uuid.New().String()
	slog.InfoContext(r.Context(), "AutoUpdate quadlets",
		slog.String("requestID", contextutils.GetRequestID(r.Context())),
		slog.String("userID", userID),
		slog.String("machineID", machineID),
		slog.String("operationID", operationID),
		slog.Bool("dryRun", dryRun))

	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute)); err != nil {
		slog.WarnContext(r.Context(), "Failed to extend write deadline", slog.Any("error", err))
	}

	streaming.SetNDJSONHeaders(w)

	ndjsonWriter, err := streaming.NewNDJSONWriter(w)
	if err != nil {
		errors.RespondWithHTTPError(w, http.StatusInternalServerError, "STREAMING_NOT_SUPPORTED", "Streaming not supported", contextutils.GetRequestID(r.Context()))
		slog.ErrorContext(r.Context(), "Streaming not supported")
		return
	}

	streaming.Run(r.Context(), ndjsonWriter, streaming.StreamConfig[AutoUpdateEvent, AutoUpdateReport]{
		Operation: func(ctx context.Context, onProgress func(AutoUpdateEvent)) (AutoUpdateReport, error) {
			return h.service.AutoUpdate(ctx, userID, machineID, dryRun, func(line sshexec.OutputLine) {
				onProgress(NewAutoUpdateLogEvent(operationID, machineID, line))
			})
		},
		OnEvent: func(event AutoUpdateEvent) error {
			return ndjsonWriter.Write(event)
		},
		CompleteEvent: func(report AutoUpdateReport) any {
			return NewAutoUpdateCompleteEvent(operationID, machineID, report)
		},
		ErrorEvent: func(err error, cancelled bool) any {
			if cancelled {
				return NewAutoUpdateErrorEvent(operationID, machineID, errors.ErrOperationCancelled.Code, errors.ErrOperationCancelled.Message)
			}
			if appErr, ok := err.(*errors.AppError); ok {
				return NewAutoUpdateErrorEvent(operationID, machineID, appErr.Code, appErr.Message)
			}
			return NewAutoUpdateErrorEvent(operationID, machineID, errors.ErrConnectionFailed.Code, errors.ErrConnectionFailed.Message)
		},
	})
}
