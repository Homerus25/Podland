package pods

import (
	"context"
	"net/http"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/web"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// PodHandler handles pod requests
type PodHandler struct {
	service *PodService
}

// NewPodHandler creates a new PodHandler
func NewPodHandler(service *PodService) *PodHandler {
	return &PodHandler{
		service: service,
	}
}

// List handles listing pods on a machine. Route-level
// RequireMachineAccess (app.go) gates the subtree; the endpoint
// envelope owns extraction, error mapping, and encoding.
func (h *PodHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (PodListResult, error) {
		// Podman's pod ps reports every pod regardless of state, so the
		// zero options are the full listing.
		pods, err := h.service.List(ctx, userID, machineID, entities.PodPSOptions{})
		if err != nil {
			return PodListResult{}, err
		}
		return ToPodListResult(pods), nil
	})
}

// Create handles creating a pod on a machine. Route-level
// RequireMachinePermission(write) (app.go) gates it at write. The body
// carries the pod's name (required), networks, named volumes and bind
// mounts, labels, and resource limits.
func (h *PodHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (*PodCreateResponse, error) {
		var request PodCreateRequest
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

		report, err := h.service.Create(ctx, userID, machineID, request)
		if err != nil {
			return nil, err
		}

		response := FromPodmanCreateReport(report, request)
		return &response, nil
	})
}

// Get handles returning one pod's detail DTO on a machine. Route-level
// RequireMachineAccess (app.go) gates the subtree at read; the endpoint
// envelope owns extraction, error mapping, and encoding. The DTO is
// mapped from podman's inspect report: name, id, state, createdAt,
// labels, restart policy, mounts, containers, and the infra config's
// networks.
func (h *PodHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (PodDetailResponse, error) {
		podID, guard := web.URLParam(r, "podId", "MISSING_POD_ID")
		if guard != nil {
			return PodDetailResponse{}, guard
		}

		report, err := h.service.Get(ctx, userID, machineID, podID)
		if err != nil {
			return PodDetailResponse{}, err
		}
		return FromPodmanInspectPod(report), nil
	})
}

// Inspect handles returning one pod's raw podman inspect report on a
// machine. Route-level RequireMachineAccess (app.go) gates the subtree
// at read; the endpoint envelope owns extraction, error mapping, and
// encoding. The report is returned unmapped: the body is podman's raw
// inspect JSON.
func (h *PodHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*entities.PodInspectReport, error) {
		podID, guard := web.URLParam(r, "podId", "MISSING_POD_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.Inspect(ctx, userID, machineID, podID)
	})
}

// Pause handles pausing every running container in a pod. Route-level
// RequireMachinePermission(write) (app.go) gates it at write. The
// service returns the pod's identity; the response DTO's state and
// timestamp are stamped here, at mapping time.
func (h *PodHandler) Pause(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*PodActionResponse, error) {
		podID, guard := web.URLParam(r, "podId", "MISSING_POD_ID")
		if guard != nil {
			return nil, guard
		}

		if err := h.service.Pause(ctx, userID, machineID, podID); err != nil {
			return nil, err
		}
		response := PodActionResponse{ID: podID, State: "paused", Timestamp: time.Now().UTC().Format(time.RFC3339)}
		return &response, nil
	})
}

// Unpause handles unpausing every paused container in a pod.
// Route-level RequireMachinePermission(write) (app.go) gates it at
// write.
func (h *PodHandler) Unpause(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*PodActionResponse, error) {
		podID, guard := web.URLParam(r, "podId", "MISSING_POD_ID")
		if guard != nil {
			return nil, guard
		}

		if err := h.service.Unpause(ctx, userID, machineID, podID); err != nil {
			return nil, err
		}
		response := PodActionResponse{ID: podID, State: "running", Timestamp: time.Now().UTC().Format(time.RFC3339)}
		return &response, nil
	})
}

// Kill handles sending podman's default signal to a pod's running
// containers. Route-level RequireMachinePermission(write) (app.go)
// gates it at write.
func (h *PodHandler) Kill(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*PodActionResponse, error) {
		podID, guard := web.URLParam(r, "podId", "MISSING_POD_ID")
		if guard != nil {
			return nil, guard
		}

		if err := h.service.Kill(ctx, userID, machineID, podID); err != nil {
			return nil, err
		}
		response := PodActionResponse{ID: podID, State: "exited", Timestamp: time.Now().UTC().Format(time.RFC3339)}
		return &response, nil
	})
}

// Restart handles restarting a pod's containers. Route-level
// RequireMachinePermission(write) (app.go) gates it at write.
func (h *PodHandler) Restart(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*PodActionResponse, error) {
		podID, guard := web.URLParam(r, "podId", "MISSING_POD_ID")
		if guard != nil {
			return nil, guard
		}

		if err := h.service.Restart(ctx, userID, machineID, podID); err != nil {
			return nil, err
		}
		response := PodActionResponse{ID: podID, State: "running", Timestamp: time.Now().UTC().Format(time.RFC3339)}
		return &response, nil
	})
}

// Start handles starting a pod's containers. Route-level
// RequireMachinePermission(write) (app.go) gates it at write. No
// timeout parameters travel — podman's defaults apply.
func (h *PodHandler) Start(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*PodActionResponse, error) {
		podID, guard := web.URLParam(r, "podId", "MISSING_POD_ID")
		if guard != nil {
			return nil, guard
		}

		if err := h.service.Start(ctx, userID, machineID, podID); err != nil {
			return nil, err
		}
		response := PodActionResponse{ID: podID, State: "running", Timestamp: time.Now().UTC().Format(time.RFC3339)}
		return &response, nil
	})
}

// Stop handles stopping a pod's containers. Route-level
// RequireMachinePermission(write) (app.go) gates it at write. No
// timeout or force parameters travel — podman's defaults apply.
func (h *PodHandler) Stop(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*PodActionResponse, error) {
		podID, guard := web.URLParam(r, "podId", "MISSING_POD_ID")
		if guard != nil {
			return nil, guard
		}

		if err := h.service.Stop(ctx, userID, machineID, podID); err != nil {
			return nil, err
		}
		response := PodActionResponse{ID: podID, State: "stopped", Timestamp: time.Now().UTC().Format(time.RFC3339)}
		return &response, nil
	})
}

// Remove handles deleting a pod; the force query flag also removes its
// running containers (default false, mirroring the container delete).
func (h *PodHandler) Remove(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, machineID string) (struct{}, error) {
		podID, guard := web.URLParam(r, "podId", "MISSING_POD_ID")
		if guard != nil {
			return struct{}{}, guard
		}

		force := web.QueryBool(r, "force", false)

		return struct{}{}, h.service.Remove(ctx, userID, machineID, podID, force)
	})
}

// Prune handles removing every stopped pod on a machine (podman's own
// selection). Bulk deletion, so write-level like the volume prune. The
// per-pod reports map into the prune response array.
func (h *PodHandler) Prune(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]PodPruneResponse, error) {
		prunes, err := h.service.Prune(ctx, userID, machineID)
		if err != nil {
			return nil, err
		}
		return FromPodmanPruneReports(prunes), nil
	})
}

// Logs handles retrieving the pod's aggregated logs: one kit call — the
// streaming kit owns the whole handler-side dispatch (the window parse,
// the query-error fallback, the param guard, the follow fork), and the
// domain contributes its param name, guard code, and two service calls.
func (h *PodHandler) Logs(w http.ResponseWriter, r *http.Request) {
	streaming.HandleLogs(w, r, streaming.LogsDispatchOp[*PodLogsResponse]{
		ParamName: "podId",
		GuardCode: "MISSING_POD_ID",
		Follow:    h.service.GetLogsWithStream,
		Buffered:  h.service.GetLogs,
	})
}
