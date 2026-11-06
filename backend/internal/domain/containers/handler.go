package containers

import (
	"context"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/bulkremove"
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
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (ContainerListResult, error) {
		opts := parseContainerListOptions(r)

		containers, err := h.service.List(ctx, userID, hostID, opts)
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
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, hostID string) (*ContainerCreateResponse, error) {
		var request ContainerCreateRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
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

		report, err := h.service.Create(ctx, userID, hostID, request)
		if err != nil {
			return nil, err
		}

		response := FromContainerCreateReport(report, request)
		return &response, nil
	})
}

// Get handles getting a container by ID
func (h *ContainerHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (ContainerGetResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return ContainerGetResponse{}, guard
		}

		containerData, err := h.service.Get(ctx, userID, hostID, containerID)
		if err != nil {
			return ContainerGetResponse{}, err
		}
		return FromPodmanInspectContainer(containerData), nil
	})
}

// Inspect handles returning a container's raw podman inspect report.
// The report is the top-level JSON body — no DTO mapping, unlike Get.
func (h *ContainerHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*entities.ContainerInspectReport, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.Inspect(ctx, userID, hostID, containerID)
	})
}

// Start handles starting a container. The lifecycle ritual and the
// action's declaration live in the domain's lifecycle module
// (lifecycle.go); the shared handler arm below maps the service's
// identity result, stamping the action's post-action state and the
// timestamp at mapping time.
func (h *ContainerHandler) Start(w http.ResponseWriter, r *http.Request) {
	h.handleLifecycle(w, r, actionStart, h.service.Start)
}

// Stop handles stopping a container, like Start above over the stop
// action's declaration.
func (h *ContainerHandler) Stop(w http.ResponseWriter, r *http.Request) {
	h.handleLifecycle(w, r, actionStop, h.service.Stop)
}

// Restart handles restarting a running container, like Start above over
// the restart action's declaration. The endpoint accepts no timeout or
// force parameters — podman's default timeout applies.
func (h *ContainerHandler) Restart(w http.ResponseWriter, r *http.Request) {
	h.handleLifecycle(w, r, actionRestart, h.service.Restart)
}

// handleLifecycle is the one handler arm for the three lifecycle
// actions: the URL-param guard, the service call, and the response
// mapping ride the endpoint envelope, and the action's declaration
// (lifecycle.go) supplies the post-action state the mapper stamps. The
// service method crosses as a function so the exported Start/Stop/
// Restart signatures stay the seam.
func (h *ContainerHandler) handleLifecycle(w http.ResponseWriter, r *http.Request, action lifecycleAction, run func(ctx context.Context, userID, hostID, containerID string) (*ContainerLifecycleResult, error)) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*ContainerLifecycleResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return nil, guard
		}

		result, err := run(ctx, userID, hostID, containerID)
		if err != nil {
			return nil, err
		}
		response := FromContainerLifecycleResult(*result, action.postState)
		return &response, nil
	})
}

// Rename handles renaming a container. The new name's shape is podman's
// to validate — the client guards only the empty and unchanged cases —
// so the request carries no name-regex tag. The response is the same
// detail DTO the GET answers, mapped from the post-rename inspect.
func (h *ContainerHandler) Rename(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (ContainerGetResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return ContainerGetResponse{}, guard
		}

		var request ContainerRenameRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return ContainerGetResponse{}, appErr
		}

		report, err := h.service.Rename(ctx, userID, hostID, containerID, request.Name)
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
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (ContainerGetResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return ContainerGetResponse{}, guard
		}

		var request ContainerUpdateRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return ContainerGetResponse{}, appErr
		}

		report, err := h.service.Update(ctx, userID, hostID, containerID, request)
		if err != nil {
			return ContainerGetResponse{}, err
		}
		return FromPodmanInspectContainer(report), nil
	})
}

// Delete handles deleting a container
func (h *ContainerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, hostID string) (struct{}, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return struct{}{}, guard
		}

		force := web.QueryBool(r, "force", false)

		return struct{}{}, h.service.Delete(ctx, userID, hostID, containerID, force)
	})
}

// BulkRemove handles removing several containers in one request. Route-level
// RequireHostPermission(write) (app.go) gates it at write. The body
// carries the container ids and the force flag (default false); the answer
// is 200 with one report entry per requested id, in request order — a
// per-id failure is an entry, never a status code. Every entry rides the
// single-item Delete's classification (internal/removal through
// internal/bulkremove), so a running container without force answers a
// CONTAINER_RUNNING entry carrying podman's message while the rest of the
// batch still processes.
func (h *ContainerHandler) BulkRemove(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) ([]bulkremove.Entry, error) {
		var request bulkremove.Request
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return nil, appErr
		}

		return bulkremove.Run(ctx, request.IDs, func(ctx context.Context, containerID string) error {
			return h.service.Delete(ctx, userID, hostID, containerID, request.Force)
		}), nil
	})
}

// Prune handles removing all stopped containers on a host. Route-level
// RequireHostPermission(write) (app.go) gates it at write; podman
// leaves running containers alone. Returns 200 with one report entry per
// removed container (ID, reclaimed size, any per-container error).
func (h *ContainerHandler) Prune(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) ([]ContainerPruneResponse, error) {
		prunes, err := h.service.Prune(ctx, userID, hostID)
		if err != nil {
			return nil, err
		}
		return FromPodmanContainerPruneReports(prunes), nil
	})
}

// Stats handles the all-stats literal route: one one-shot snapshot for
// every running container on the host. Nothing running is a legal
// empty snapshot (200 with an empty array), and the route stays at the
// subtree's read level like list/get/logs.
func (h *ContainerHandler) Stats(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (ContainerStatsResult, error) {
		stats, err := h.service.Stats(ctx, userID, hostID, nil, true)
		if err != nil {
			return ContainerStatsResult{}, err
		}
		return ToContainerStatsResult(stats), nil
	})
}

// ContainerStats handles the per-container stats subroute: the one
// container's snapshot as a single DTO. A container without stats —
// unknown or known-but-not-running, which podman's own stats handler
// refuses alike — answers the domain's 404 sentinel.
func (h *ContainerHandler) ContainerStats(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (ContainerStatsResponse, error) {
		containerID, guard := web.URLParam(r, "containerId", "MISSING_CONTAINER_ID")
		if guard != nil {
			return ContainerStatsResponse{}, guard
		}

		stats, err := h.service.Stats(ctx, userID, hostID, []string{containerID}, false)
		if err != nil {
			return ContainerStatsResponse{}, err
		}
		if len(stats) == 0 {
			// Podman's stats handler maps any first-report error to
			// 404; a clean empty answer for a targeted container (a
			// race between running and sampled) gets the same
			// terminal rather than a fabricated zero DTO.
			return ContainerStatsResponse{}, appErrors.ErrContainerNotFound
		}
		return FromPodmanContainerStats(stats[0]), nil
	})
}

// Logs handles retrieving logs for a container: one kit call — the
// streaming kit owns the whole handler-side dispatch (the window parse,
// the query-error fallback, the param guard, the follow fork), and the
// domain contributes its param name, guard code, and two service calls.
func (h *ContainerHandler) Logs(w http.ResponseWriter, r *http.Request) {
	streaming.HandleLogs(w, r, streaming.LogsDispatchOp[*ContainerLogsResponse]{
		ParamName: "containerId",
		GuardCode: "MISSING_CONTAINER_ID",
		Follow:    h.service.GetLogsWithStream,
		Buffered:  h.service.GetLogs,
	})
}
