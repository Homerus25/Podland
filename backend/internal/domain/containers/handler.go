package containers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

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
	service *ContainerService
}

// NewContainerHandler creates a new ContainerHandler
func NewContainerHandler(service *ContainerService) *ContainerHandler {
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

// Logs handles retrieving logs for a container. The query is parsed once:
// only a clean follow request crosses the streaming envelope, while query
// errors ride the JSON envelope even when follow=true was requested, so
// no code path here writes an error response by hand.
func (h *ContainerHandler) Logs(w http.ResponseWriter, r *http.Request) {
	since, tail, queryErr := streaming.ParseLogWindow(r)
	if queryErr != nil {
		web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (struct{}, error) {
			return struct{}{}, queryErr
		})
		return
	}

	containerID := chi.URLParam(r, "containerId")

	if streaming.ParseFollow(r) && containerID != "" {
		slog.InfoContext(r.Context(), "streamLogs", slog.String("containerID", containerID), slog.Bool("follow", true))
		streaming.RunLogStream(w, r, streaming.LogStreamOp{
			Since: since,
			Tail:  tail,
			Stream: func(ctx context.Context, userID, machineID string, onEvent func(streaming.LogEvent)) error {
				return h.service.GetLogsWithStream(ctx, userID, machineID, containerID, since, tail, onEvent)
			},
		})
		return
	}

	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*ContainerLogsResponse, error) {
		if containerID == "" {
			return nil, errors.NewAppError("MISSING_CONTAINER_ID", "Container ID is required", http.StatusBadRequest)
		}
		return h.service.GetLogs(ctx, userID, machineID, containerID, since, tail)
	})
}
