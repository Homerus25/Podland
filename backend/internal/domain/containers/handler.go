package containers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// ContainerHandler handles container requests
//
// JSON endpoints cross the endpoint envelope (internal/web): extraction,
// error envelope, and encoding live there. Follow-mode logs cross the
// streaming envelope (internal/streaming): Begin + Run own headers,
// identity, disconnect, and the terminal-event mapping.
type ContainerHandler struct {
	service ContainerServiceInterface
}

// NewContainerHandler creates a new ContainerHandler
func NewContainerHandler(service ContainerServiceInterface) *ContainerHandler {
	return &ContainerHandler{
		service: service,
	}
}

// List handles listing containers
func (h *ContainerHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (ContainerListResult, error) {
		opts := parseContainerListOptions(r)

		containers, err := h.service.List(ctx, userID, machineID, opts)
		if err != nil {
			return ContainerListResult{}, err
		}

		containersAny := make([]any, len(containers))
		for i, c := range containers {
			containersAny[i] = c
		}
		return ToContainerListResult(containersAny), nil
	})
}

// parseContainerListOptions reads the list query parameters podman
// understands (all, filters, size, namespace, sync, external, last,
// latest). Malformed values are ignored, matching the endpoints'
// documented behavior.
func parseContainerListOptions(r *http.Request) entities.ContainerListOptions {
	opts := entities.ContainerListOptions{}

	if allParam := r.URL.Query().Get("all"); allParam != "" {
		if all, err := strconv.ParseBool(allParam); err == nil {
			opts.All = all
		}
	}

	if filtersParam := r.URL.Query().Get("filters"); filtersParam != "" {
		opts.Filters = make(map[string][]string)
		opts.Filters["custom"] = []string{filtersParam}
	}

	if sizeParam := r.URL.Query().Get("size"); sizeParam != "" {
		if size, err := strconv.ParseBool(sizeParam); err == nil {
			opts.Size = size
		}
	}

	if namespaceParam := r.URL.Query().Get("namespace"); namespaceParam != "" {
		if namespace, err := strconv.ParseBool(namespaceParam); err == nil {
			opts.Namespace = namespace
		}
	}

	if syncParam := r.URL.Query().Get("sync"); syncParam != "" {
		if sync, err := strconv.ParseBool(syncParam); err == nil {
			opts.Sync = sync
		}
	}

	if externalParam := r.URL.Query().Get("external"); externalParam != "" {
		if external, err := strconv.ParseBool(externalParam); err == nil {
			opts.External = external
		}
	}

	if lastParam := r.URL.Query().Get("last"); lastParam != "" {
		if last, err := strconv.Atoi(lastParam); err == nil {
			opts.Last = last
		}
	}

	if latestParam := r.URL.Query().Get("latest"); latestParam != "" {
		if latest, err := strconv.ParseBool(latestParam); err == nil {
			opts.Latest = latest
		}
	}

	return opts
}

// Create handles creating a container
func (h *ContainerHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (*ContainerCreateResponse, error) {
		var request ContainerCreateRequest
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

// Get handles getting a container by ID
func (h *ContainerHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (ContainerGetResponse, error) {
		containerID := chi.URLParam(r, "containerId")
		if containerID == "" {
			return ContainerGetResponse{}, errors.NewAppError("MISSING_CONTAINER_ID", "Container ID is required", http.StatusBadRequest)
		}

		containerData, err := h.service.Get(ctx, userID, machineID, containerID)
		if err != nil {
			return ContainerGetResponse{}, err
		}
		return FromPodmanInspectContainer(containerData), nil
	})
}

// Start handles starting a container
func (h *ContainerHandler) Start(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*ContainerStartResponse, error) {
		containerID := chi.URLParam(r, "containerId")
		if containerID == "" {
			return nil, errors.NewAppError("MISSING_CONTAINER_ID", "Container ID is required", http.StatusBadRequest)
		}

		return h.service.Start(ctx, userID, machineID, containerID)
	})
}

// Stop handles stopping a container
func (h *ContainerHandler) Stop(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*ContainerStopResponse, error) {
		containerID := chi.URLParam(r, "containerId")
		if containerID == "" {
			return nil, errors.NewAppError("MISSING_CONTAINER_ID", "Container ID is required", http.StatusBadRequest)
		}

		return h.service.Stop(ctx, userID, machineID, containerID)
	})
}

// Delete handles deleting a container
func (h *ContainerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, machineID string) (struct{}, error) {
		containerID := chi.URLParam(r, "containerId")
		if containerID == "" {
			return struct{}{}, errors.NewAppError("MISSING_CONTAINER_ID", "Container ID is required", http.StatusBadRequest)
		}

		// A malformed force flag is ignored (deletes as unforced),
		// matching the documented behavior of this endpoint.
		force := false
		if forceParam := r.URL.Query().Get("force"); forceParam != "" {
			if parsed, err := strconv.ParseBool(forceParam); err == nil {
				force = parsed
			}
		}

		return struct{}{}, h.service.Delete(ctx, userID, machineID, containerID, force)
	})
}

// Logs handles retrieving logs for a container
func (h *ContainerHandler) Logs(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Logs")

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
		h.streamLogs(w, r, containerID, since, tail)
		return
	}

	h.bufferedLogs(w, r, containerID, since, tail)
}

// bufferedLogs serves follow=false: one JSON document with the logs
// collected so far. Identity checks stay here (the streaming branch gets
// them from the streaming envelope instead).
func (h *ContainerHandler) bufferedLogs(w http.ResponseWriter, r *http.Request, containerID string, since time.Time, tail int64) {
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

// streamLogs serves follow=true through the streaming envelope: Begin owns
// headers, writer, and identity; Run owns the select loop, disconnect
// cancellation, and the error mapping. The handler is config plus event
// vocabulary — writes happen only inside Run's loop, never from the
// service goroutine, so write-after-return is impossible by construction.
func (h *ContainerHandler) streamLogs(w http.ResponseWriter, r *http.Request, containerID string, since time.Time, tail int64) {
	slog.InfoContext(r.Context(), "streamLogs", slog.String("containerID", containerID), slog.Bool("follow", true))

	stream := streaming.Begin(w, r)
	if stream == nil {
		return
	}

	streaming.Run(r.Context(), stream.Writer, streaming.StreamConfig[ContainerLogEvent, struct{}]{
		Operation: func(ctx context.Context, onProgress func(ContainerLogEvent)) (struct{}, error) {
			return struct{}{}, h.service.GetLogsWithStream(ctx, stream.UserID, stream.MachineID, containerID, since, tail, onProgress)
		},
		OnEvent: func(event ContainerLogEvent) error {
			return stream.Writer.Write(event)
		},
		CompleteEvent: func(struct{}) any {
			return ContainerLogEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     streaming.StreamEndedLevel,
				Message:   streaming.StreamEndedMessage,
			}
		},
		ErrorDefault: errors.ErrConnectionFailed,
		ErrorEvent:   logStreamTerminalEvent,
	})
}

// logStreamTerminalEvent stamps the module-mapped (code, message,
// cancelled) into the log-event vocabulary: "CODE: human message" at ERROR
// level, WARN when the stream was cancelled.
func logStreamTerminalEvent(code, message string, cancelled bool) any {
	level := "ERROR"
	if cancelled {
		level = "WARN"
	}
	return ContainerLogEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Message:   code + ": " + message,
	}
}
