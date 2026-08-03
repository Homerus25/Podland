package quadlets

import (
	"strings"

	"go.podman.io/podman/v6/pkg/domain/entities"
)

// QuadletListResponse represents a quadlet in the API list response.
type QuadletListResponse struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// Name of the quadlet
	Name string `json:"name"`
	// Path to the quadlet file
	Path string `json:"path"`
	// Systemd unit name
	Unit string `json:"unit"`
	// Resource type (container, pod, image, volume, network)
	Resource string `json:"resource"`
	// State of the quadlet (unknown, stopped, running, error)
	State string `json:"state"`
	// Raw quadlet file content (optional)
	Raw string `json:"raw,omitempty"`
}

// FromListQuadlet converts a podman entities.ListQuadlet to QuadletListResponse.
func FromListQuadlet(quadlet *entities.ListQuadlet) QuadletListResponse {
	if quadlet == nil {
		return QuadletListResponse{}
	}

	return QuadletListResponse{
		ID:       quadlet.Name,
		Name:     quadlet.Name,
		Path:     quadlet.Path,
		Unit:     quadlet.UnitName,
		Resource: deriveResourceFromPath(quadlet.Path),
		State:    parseStateFromStatus(quadlet.Status),
		Raw:      "", // Raw content not included in list response
	}
}

// ToQuadletListResponse converts a slice of podman ListQuadlet to a slice of QuadletListResponse.
func ToQuadletListResponse(quadlets []*entities.ListQuadlet) []QuadletListResponse {
	result := make([]QuadletListResponse, len(quadlets))
	for i, quadlet := range quadlets {
		result[i] = FromListQuadlet(quadlet)
	}
	return result
}

// deriveResourceFromPath derives the resource type from the quadlet file path.
func deriveResourceFromPath(path string) string {
	if path == "" {
		return "unknown"
	}

	lowerPath := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lowerPath, ".container"):
		return "container"
	case strings.HasSuffix(lowerPath, ".pod"):
		return "pod"
	case strings.HasSuffix(lowerPath, ".image"):
		return "image"
	case strings.HasSuffix(lowerPath, ".volume"):
		return "volume"
	case strings.HasSuffix(lowerPath, ".network"):
		return "network"
	case strings.HasSuffix(lowerPath, ".kube"):
		return "container" // Map kube to closest contract value
	case strings.HasSuffix(lowerPath, ".build"):
		return "image" // Map build to closest contract value
	default:
		return "unknown"
	}
}

// parseStateFromStatus parses the systemd status string into a normalized state.
func parseStateFromStatus(status string) string {
	if status == "" {
		return "unknown"
	}

	lowerStatus := strings.ToLower(status)
	switch {
	case strings.Contains(lowerStatus, "running"):
		return "running"
	case strings.Contains(lowerStatus, "stopped"):
		return "stopped"
	case strings.Contains(lowerStatus, "failed") || strings.Contains(lowerStatus, "error"):
		return "error"
	default:
		return "unknown"
	}
}