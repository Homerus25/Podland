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

// ToVolumeListResponse converts podman volume reports to the API array
func ToVolumeListResponse(reports []*entities.VolumeListReport) []VolumeResponse {
	out := make([]VolumeResponse, 0, len(reports))
	for _, report := range reports {
		out = append(out, FromPodmanVolume(report))
	}
	return out
}
