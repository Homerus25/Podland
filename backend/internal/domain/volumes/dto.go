package volumes

import (
	"time"

	"go.podman.io/podman/v6/pkg/domain/entities"
)

// VolumeResponse represents a volume in the list API response
type VolumeResponse struct {
	// Volume name
	Name string `json:"name"`
	// Volume driver
	Driver string `json:"driver"`
	// Time when the volume was created (RFC3339)
	CreatedAt string `json:"createdAt"`
	// Whether the volume was created anonymously
	Anonymous bool `json:"anonymous"`
	// User-defined key/value metadata
	Labels map[string]string `json:"labels,omitempty"`
	// Driver options used when creating the volume
	Options map[string]string `json:"options,omitempty"`
	// Mountpoint of the volume on the machine
	Mountpoint string `json:"mountpoint"`
	// Size/usage, when podman reports it
	Size *int64 `json:"size,omitempty"`
}

// FromPodmanVolume converts a podman VolumeListReport to the API response
func FromPodmanVolume(report *entities.VolumeListReport) VolumeResponse {
	if report == nil {
		return VolumeResponse{}
	}
	response := VolumeResponse{
		Name:       report.Name,
		Driver:     report.Driver,
		CreatedAt:  report.CreatedAt.UTC().Format(time.RFC3339),
		Anonymous:  report.Anonymous,
		Labels:     report.Labels,
		Options:    report.Options,
		Mountpoint: report.Mountpoint,
	}
	if response.Labels == nil {
		response.Labels = map[string]string{}
	}
	if response.Options == nil {
		response.Options = map[string]string{}
	}
	return response
}

// VolumeCreateRequest is the POST /api/machines/{id}/volumes body.
// Name is optional: an empty name creates an anonymous volume whose
// generated name podman reports back. Name-format validation is podman's
// job — invalid names surface as client errors from the service.
type VolumeCreateRequest struct {
	// Volume name; empty means podman generates one (anonymous volume)
	Name string `json:"name"`
	// User-defined key/value metadata
	Labels map[string]string `json:"labels,omitempty"`
	// Volume driver; podman defaults to its "local" driver
	Driver string `json:"driver,omitempty"`
	// Mapping of driver options and values
	Options map[string]string `json:"options,omitempty"`
}

// ToPodmanVolumeCreateOptions maps the API request to podman's create
// options.
func ToPodmanVolumeCreateOptions(req VolumeCreateRequest) entities.VolumeCreateOptions {
	return entities.VolumeCreateOptions{
		Name:    req.Name,
		Labels:  req.Labels,
		Driver:  req.Driver,
		Options: req.Options,
	}
}

// VolumeInspectResponse represents a single volume's full config in the API response
type VolumeInspectResponse struct {
	// Volume name
	Name string `json:"name"`
	// Volume driver
	Driver string `json:"driver"`
	// Time when the volume was created (RFC3339)
	CreatedAt string `json:"createdAt"`
	// Whether the volume was created anonymously
	Anonymous bool `json:"anonymous"`
	// User-defined key/value metadata
	Labels map[string]string `json:"labels,omitempty"`
	// Driver options used when creating the volume
	Options map[string]string `json:"options,omitempty"`
	// Mountpoint of the volume on the machine
	Mountpoint string `json:"mountpoint"`
	// Size/usage, when podman reports it
	Size *int64 `json:"size,omitempty"`
}

// FromPodmanVolumeInspect converts a podman VolumeInspectReport to the API response
func FromPodmanVolumeInspect(report *entities.VolumeInspectReport) VolumeInspectResponse {
	if report == nil || report.VolumeConfigResponse == nil {
		return VolumeInspectResponse{}
	}
	cfg := report.VolumeConfigResponse
	response := VolumeInspectResponse{
		Name:       cfg.Name,
		Driver:     cfg.Driver,
		CreatedAt:  cfg.CreatedAt.UTC().Format(time.RFC3339),
		Anonymous:  cfg.Anonymous,
		Labels:     cfg.Labels,
		Options:    cfg.Options,
		Mountpoint: cfg.Mountpoint,
	}
	if response.Labels == nil {
		response.Labels = map[string]string{}
	}
	if response.Options == nil {
		response.Options = map[string]string{}
	}
	return response
}

// ToVolumeListResponse converts podman volume reports to the API array
func ToVolumeListResponse(reports []*entities.VolumeListReport) []VolumeResponse {
	out := make([]VolumeResponse, 0, len(reports))
	for _, report := range reports {
		out = append(out, FromPodmanVolume(report))
	}
	return out
}
