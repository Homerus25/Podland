package pods

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"go.podman.io/podman/v6/pkg/domain/entities"
)

// PodListResponse represents a pod in the list API response: the fields
// the pods tab renders — name, state, container count, networks, and
// creation date.
type PodListResponse struct {
	// Pod ID
	ID string `json:"id"`
	// Pod name
	Name string `json:"name"`
	// Pod state (podman's status: Running, Exited, Created, Paused, Degraded, Stopped)
	State string `json:"state"`
	// Time when the pod was created (RFC3339)
	CreatedAt string `json:"createdAt"`
	// Number of containers in the pod, infra container included
	ContainerCount int `json:"containerCount"`
	// Network names connected to the pod's infra container
	Networks []string `json:"networks,omitempty"`
	// Pod labels
	Labels map[string]string `json:"labels,omitempty"`
}

// PodCreateRequest is the POST /api/hosts/{id}/pods body: the pod's
// name, the networks it joins, named volumes, bind mounts, and port
// mappings in the same shapes as the container create request, labels,
// and resource limits.
type PodCreateRequest struct {
	// Name of the pod (required)
	Name string `json:"name" validate:"required,min=1,max=255"`
	// Networks to attach the pod's infra container to (optional)
	Networks []string `json:"networks,omitempty"`
	// Named volumes to mount into the pod (optional)
	Volumes []VolumeRequest `json:"volumes,omitempty" validate:"omitempty,dive"`
	// Bind mounts (optional)
	Mounts []MountRequest `json:"mounts,omitempty" validate:"omitempty,dive"`
	// Port mappings published on the pod's infra container (optional)
	PortMappings []PortBindingRequest `json:"portMappings,omitempty" validate:"omitempty,dive"`
	// Labels for the pod (optional)
	Labels map[string]string `json:"labels,omitempty"`
	// CPU limit in cores (optional)
	CPUs float64 `json:"cpus,omitempty"`
	// Memory limit, podman units syntax ("512m", "1g") (optional)
	Memory string `json:"memory,omitempty"`
}

// VolumeRequest represents a named volume to mount into the pod — the
// same shape as the container create request's volume entry.
type VolumeRequest struct {
	// Name of the named volume (required)
	Name string `json:"name" validate:"required,min=1"`
	// Destination path inside the pod (required)
	Destination string `json:"destination" validate:"required,min=1"`
	// Mount mode (optional, defaults to rw)
	Mode string `json:"mode,omitempty" validate:"omitempty,oneof=ro rw"`
}

// MountRequest represents a bind mount into the pod — the same shape as
// the container create request's mount entry.
type MountRequest struct {
	Source      string `json:"source" validate:"required,min=1"`
	Destination string `json:"destination" validate:"required,min=1"`
	Mode        string `json:"mode,omitempty" validate:"omitempty,oneof=ro rw"`
	// SELinux shared relabel (z): the mounted content is shared between
	// containers (optional). Mutually exclusive with SelinuxPrivate —
	// enforced by an explicit check in the handler next to
	// validate.Struct, mirroring the containers create handler.
	SelinuxShared bool `json:"selinuxShared,omitempty"`
	// SELinux private relabel (Z): only this pod may access the mounted
	// content (optional). Mutually exclusive with SelinuxShared.
	SelinuxPrivate bool `json:"selinuxPrivate,omitempty"`
	// Chown the mounted content to the container's user (U) (optional)
	ChownUser bool `json:"chownUser,omitempty"`
}

// PortBindingRequest represents a port mapping to publish on the pod's
// infra container — the same shape as the container create request's
// port binding entry.
type PortBindingRequest struct {
	// Port on the host (required, 1-65535)
	HostPort int `json:"hostPort" validate:"required,min=1,max=65535"`
	// Port inside the pod (required, 1-65535)
	ContainerPort int `json:"containerPort" validate:"required,min=1,max=65535"`
	// Protocol (optional, defaults to tcp)
	Protocol string `json:"protocol,omitempty" validate:"omitempty,oneof=tcp udp"`
}

// PodCreateResponse represents the response after creating a pod,
// mirroring the container create response minus the image fields.
type PodCreateResponse struct {
	// Pod ID
	ID string `json:"id"`
	// Pod name
	Name string `json:"name"`
	// Pod state
	State string `json:"state"`
	// Creation timestamp
	CreatedAt string `json:"createdAt"`
}

// FromPodmanCreateReport maps podman's create report and the create
// request to the create response DTO. The report carries only the pod
// ID; the name comes from the request, and the state and timestamp are
// stamped at mapping time.
func FromPodmanCreateReport(report *entities.PodCreateReport, request PodCreateRequest) PodCreateResponse {
	return PodCreateResponse{
		ID:        report.Id,
		Name:      request.Name,
		State:     "created",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// PodListResult wraps the list of pods with metadata, mirroring the
// containers list envelope.
type PodListResult struct {
	// List of pods
	Pods []PodListResponse `json:"pods"`
	// Total count of pods
	Total int `json:"total"`
	// Request timestamp
	Timestamp string `json:"timestamp"`
}

// FromPodmanListPod converts a podman ListPodsReport entity to
// PodListResponse. Typed at both ends, mirroring the containers, volumes,
// and images list mappers (backend-conventions, candidate C22).
func FromPodmanListPod(report *entities.ListPodsReport) PodListResponse {
	if report == nil {
		return PodListResponse{}
	}
	return PodListResponse{
		ID:             report.Id,
		Name:           report.Name,
		State:          report.Status,
		CreatedAt:      report.Created.UTC().Format(time.RFC3339),
		ContainerCount: len(report.Containers),
		Networks:       report.Networks,
		Labels:         report.Labels,
	}
}

// ToPodListResult converts a list of podman pod reports to
// PodListResult. This function handles the complete transformation from
// service layer to API response.
func ToPodListResult(reports []*entities.ListPodsReport) PodListResult {
	result := PodListResult{
		Pods:      make([]PodListResponse, 0, len(reports)),
		Total:     len(reports),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	for _, report := range reports {
		result.Pods = append(result.Pods, FromPodmanListPod(report))
	}

	return result
}

// PodContainerRef is one container in the pod detail DTO: the
// container's ID, name, and state.
type PodContainerRef struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state
	State string `json:"state"`
}

// PodDetailResponse is the GET /api/hosts/{id}/pods/{podId} detail
// DTO: the fields the pod detail page renders, mapped from the inspect
// report.
type PodDetailResponse struct {
	// Pod ID
	ID string `json:"id"`
	// Pod name
	Name string `json:"name"`
	// Pod state (podman's status: Running, Exited, Created, Paused, Degraded, Stopped)
	State string `json:"state"`
	// Time when the pod was created (RFC3339)
	CreatedAt string `json:"createdAt"`
	// Pod labels
	Labels map[string]string `json:"labels,omitempty"`
	// Pod restart policy (podman's effective policy)
	RestartPolicy string `json:"restartPolicy,omitempty"`
	// Mounts as --mount-style strings ("type=…,source=…,destination=…"),
	// named volumes and binds alike
	Mounts []string `json:"mounts,omitempty"`
	// Containers in the pod (id, name, state)
	Containers []PodContainerRef `json:"containers"`
	// Network names connected to the pod's infra container
	Networks []string `json:"networks,omitempty"`
}

// FromPodmanInspectPod converts a podman PodInspectReport entity to the
// detail DTO. A nil report maps to the zero DTO; containers are ordered
// by ID so the answer is stable across calls.
func FromPodmanInspectPod(report *entities.PodInspectReport) PodDetailResponse {
	if report == nil || report.InspectPodData == nil {
		return PodDetailResponse{
			Containers: []PodContainerRef{},
		}
	}

	mounts := make([]string, 0, len(report.Mounts))
	for _, mount := range report.Mounts {
		mounts = append(mounts, fmt.Sprintf("type=%s,source=%s,destination=%s", mount.Type, mount.Source, mount.Destination))
	}

	containers := make([]PodContainerRef, 0, len(report.Containers))
	for _, container := range report.Containers {
		containers = append(containers, PodContainerRef{
			ID:    container.ID,
			Name:  container.Name,
			State: container.State,
		})
	}
	slices.SortFunc(containers, func(a, b PodContainerRef) int {
		return strings.Compare(a.ID, b.ID)
	})

	var networks []string
	if report.InfraConfig != nil {
		networks = report.InfraConfig.Networks
	}

	return PodDetailResponse{
		ID:            report.ID,
		Name:          report.Name,
		State:         report.State,
		CreatedAt:     report.Created.UTC().Format(time.RFC3339),
		Labels:        report.Labels,
		RestartPolicy: report.RestartPolicy,
		Mounts:        mounts,
		Containers:    containers,
		Networks:      networks,
	}
}

// PodActionResponse is a lifecycle action endpoint's response body: the
// pod's ID, its state after the action (stamped at mapping time), and
// the response timestamp.
type PodActionResponse struct {
	// Pod ID
	ID string `json:"id"`
	// Pod state after the action
	State string `json:"state"`
	// Response timestamp
	Timestamp string `json:"timestamp"`
}

// PodPruneResponse is one entry of the prune response: the removed
// pod's ID, or the per-pod error when podman could not remove it.
type PodPruneResponse struct {
	// ID of the removed pod
	ID string `json:"id"`
	// Per-pod error when podman could not remove the pod
	Error string `json:"error,omitempty"`
}

// FromPodmanPruneReports maps podman's prune reports to the prune
// response entries. Nil reports are skipped; pruning with nothing to
// remove yields an empty array, not nil.
func FromPodmanPruneReports(reports []*entities.PodPruneReport) []PodPruneResponse {
	entries := make([]PodPruneResponse, 0, len(reports))
	for _, report := range reports {
		if report == nil {
			continue
		}
		entry := PodPruneResponse{ID: report.Id}
		if report.Err != nil {
			entry.Error = report.Err.Error()
		}
		entries = append(entries, entry)
	}
	return entries
}

// PodLogsResponse represents the response for pod logs
type PodLogsResponse struct {
	// Pod ID
	ID string `json:"id"`
	// Pod logs content (multiline string; podman's aggregated output
	// across every container in the pod, infra included)
	Logs string `json:"logs"`
}
