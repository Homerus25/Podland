package containers

import (
	"context"
	"log/slog"
	"net/http"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/web"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// ContainerHandler handles container requests
//
// JSON endpoints cross the endpoint envelope (internal/web): extraction,
// error envelope, and encoding live there. Follow-mode logs cross the
// streaming kit's log seam (internal/streaming): RunLogStream owns
// headers, identity, disconnect, and the terminal-event mapping.
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

		return ToContainerListResult(containers), nil
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

		// A mount carrying both SELinux labels is refused next to
		// validate.Struct (the volumes populate handler's pattern):
		// go-playground/validator has no cross-field tag, and podman's
		// z and Z labels are mutually exclusive by definition.
		for _, mount := range request.Mounts {
			if mount.SelinuxShared && mount.SelinuxPrivate {
				return nil, appErrors.ErrValidationFailed.WithMessage("mounts: selinuxShared and selinuxPrivate are mutually exclusive")
			}
		}

		report, err := h.service.Create(ctx, userID, machineID, request)
		if err != nil {
			return nil, err
		}

		response := FromContainerCreateReport(report, request)
		return &response, nil
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

// Inspect handles returning a container's raw podman inspect report.
// The report is the top-level JSON body — no DTO mapping, unlike Get.
func (h *ContainerHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*entities.ContainerInspectReport, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.Inspect(ctx, userID, machineID, containerID)
	})
}

// Start handles starting a container. The service returns the
// container's identity; the response DTO's state and timestamp are
// stamped here, at mapping time.
func (h *ContainerHandler) Start(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*ContainerStartResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return nil, guard
		}

		result, err := h.service.Start(ctx, userID, machineID, containerID)
		if err != nil {
			return nil, err
		}
		response := FromContainerStartResult(*result)
		return &response, nil
	})
}

// Stop handles stopping a container. The service returns the
// container's identity; the response DTO's state and timestamp are
// stamped here, at mapping time.
func (h *ContainerHandler) Stop(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*ContainerStopResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return nil, guard
		}

		result, err := h.service.Stop(ctx, userID, machineID, containerID)
		if err != nil {
			return nil, err
		}
		response := FromContainerStopResult(*result)
		return &response, nil
	})
}

// Restart handles restarting a running container. The service returns
// the container's identity; the response DTO's state and timestamp are
// stamped here, at mapping time. The endpoint accepts no timeout or
// force parameters — podman's default timeout applies.
func (h *ContainerHandler) Restart(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*ContainerRestartResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return nil, guard
		}

		result, err := h.service.Restart(ctx, userID, machineID, containerID)
		if err != nil {
			return nil, err
		}
		response := FromContainerRestartResult(*result)
		return &response, nil
	})
}

// Rename handles renaming a container. The new name's shape is podman's
// to validate — the client guards only the empty and unchanged cases —
// so the request carries no name-regex tag. The response is the same
// detail DTO the GET answers, mapped from the post-rename inspect.
func (h *ContainerHandler) Rename(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (ContainerGetResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return ContainerGetResponse{}, guard
		}

		var request ContainerRenameRequest
		if appErr := web.Decode(r, &request); appErr != nil {
			return ContainerGetResponse{}, appErr
		}

		report, err := h.service.Rename(ctx, userID, machineID, containerID, request.Name)
		if err != nil {
			return ContainerGetResponse{}, err
		}
		return FromPodmanInspectContainer(report), nil
	})
}

// Update handles changing a container's resource limits and healthcheck
// configuration. The response is the same detail DTO the GET answers,
// mapped from the post-update inspect — one round trip carries the new
// state. Patch semantics: omitted request fields mean "unchanged"; the
// guards that cannot be struct tags live in the update builder behind
// the service.
func (h *ContainerHandler) Update(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (ContainerGetResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return ContainerGetResponse{}, guard
		}

		var request ContainerUpdateRequest
		if appErr := web.Decode(r, &request); appErr != nil {
			return ContainerGetResponse{}, appErr
		}

		report, err := h.service.Update(ctx, userID, machineID, containerID, request)
		if err != nil {
			return ContainerGetResponse{}, err
		}
		return FromPodmanInspectContainer(report), nil
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

// Prune handles removing all stopped containers on a machine. Route-level
// RequireMachinePermission(write) (app.go) gates it at write; podman
// leaves running containers alone. Returns 200 with one report entry per
// removed container (ID, reclaimed size, any per-container error).
func (h *ContainerHandler) Prune(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]ContainerPruneResponse, error) {
		prunes, err := h.service.Prune(ctx, userID, machineID)
		if err != nil {
			return nil, err
		}
		return FromPodmanContainerPruneReports(prunes), nil
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
