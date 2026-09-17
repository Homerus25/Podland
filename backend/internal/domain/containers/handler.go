package containers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/web"
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
// latest). The bool/int flags cross web's lenient query helpers:
// malformed values fall back to the zero defaults.
func parseContainerListOptions(r *http.Request) entities.ContainerListOptions {
	opts := entities.ContainerListOptions{}
	opts.All = web.QueryBool(r, "all", false)

	if filtersParam := r.URL.Query().Get("filters"); filtersParam != "" {
		opts.Filters = make(map[string][]string)
		opts.Filters["custom"] = []string{filtersParam}
	}

	opts.Size = web.QueryBool(r, "size", false)
	opts.Namespace = web.QueryBool(r, "namespace", false)
	opts.Sync = web.QueryBool(r, "sync", false)
	opts.External = web.QueryBool(r, "external", false)
	opts.Last = web.QueryInt(r, "last", 0)
	opts.Latest = web.QueryBool(r, "latest", false)

	return opts
}

// Create handles creating a container
func (h *ContainerHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (*ContainerCreateResponse, error) {
		var request ContainerCreateRequest
		if appErr := web.Decode(r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.Create(ctx, userID, machineID, request)
	})
}

// Get handles getting a container by ID
func (h *ContainerHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (ContainerGetResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return ContainerGetResponse{}, guard
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
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.Start(ctx, userID, machineID, containerID)
	})
}

// Stop handles stopping a container
func (h *ContainerHandler) Stop(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*ContainerStopResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.Stop(ctx, userID, machineID, containerID)
	})
}

// Delete handles deleting a container
func (h *ContainerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, machineID string) (struct{}, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return struct{}{}, guard
		}

		force := web.QueryBool(r, "force", false)

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

	containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")

	if guard == nil && streaming.ParseFollow(r) {
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
		if guard != nil {
			return nil, guard
		}
		return h.service.GetLogs(ctx, userID, machineID, containerID, since, tail)
	})
}
