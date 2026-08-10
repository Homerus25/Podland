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

// QuadletGetResponse represents a quadlet in the API get response.
type QuadletGetResponse struct {
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
	// Raw quadlet file content
	Content string `json:"content"`
}

// QuadletUpdateRequest represents a request to replace an existing quadlet's
// content. Only the raw file content is sent; the quadlet name comes from the
// URL path and the resource type is derived from the existing quadlet's path
// during the existence check.
type QuadletUpdateRequest struct {
	// Raw quadlet file content. Required.
	Content string `json:"content" validate:"required,min=1"`
}

// QuadletCreateRequest represents a request to create a new quadlet.
type QuadletCreateRequest struct {
	// Name of the quadlet (without extension). Required.
	Name string `json:"name" validate:"required,min=1"`
	// Resource type. Must be one of: container, pod, image, volume, network. Required.
	Resource string `json:"resource" validate:"required,oneof=container pod image volume network"`
	// Raw quadlet file content. Required.
	Content string `json:"content" validate:"required,min=1"`
}

// QuadletCreateResponse represents a quadlet in the API create response.
// Per the epic's no-persistence ruling, generation/createdAt/updatedAt are
// omitted (documented deviation from the contract at doku/description_api.md:914-925).
type QuadletCreateResponse struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// Name of the quadlet (without extension)
	Name string `json:"name"`
	// Path to the installed quadlet file (populated when podman reports it)
	Path string `json:"path,omitempty"`
	// Systemd unit name (often empty immediately after install before systemd reload)
	Unit string `json:"unit,omitempty"`
	// Resource type (container, pod, image, volume, network)
	Resource string `json:"resource"`
	// State of the quadlet. Newly installed quadlets are reported as "unknown"
	// until systemd reload completes and reports a status.
	State string `json:"state"`
	// The raw quadlet file content that was installed (echoed back from the request)
	Content string `json:"content"`
}

// LifecycleOptions carries the optional force/timeout parameters for stop and
// restart operations (doku/description_api.md:1019-1022, 1048-1050). Start has
// no options per the contract.
type LifecycleOptions struct {
	// Force requests an aggressive stop/restart. Maps to `systemctl --force`.
	Force bool
	// Timeout is the stop/restart grace period in seconds. 0 means unset.
	// Maps to `systemctl --kill-seconds=<n>`.
	Timeout int
}

// QuadletLifecycleResponse is the success body for the start/stop/restart
// endpoints (doku/description_api.md:994-1001, 1023-1030, 1052-1060). The shape
// is intentionally narrow: id + state + unit, with restartedAt on restart only.
type QuadletLifecycleResponse struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// State of the quadlet after the operation (running / stopped)
	State string `json:"state"`
	// Systemd unit name that was acted on
	Unit string `json:"unit"`
	// RestartedAt is the timestamp of a restart (restart only, ISO8601).
	RestartedAt string `json:"restartedAt,omitempty"`
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

// resourceToExtension maps a resource type to its quadlet filename extension.
// This is the inverse of deriveResourceFromPath for the contract's resource enum
// (container, pod, image, volume, network). Unknown resources default to ".container".
func resourceToExtension(resource string) string {
	switch strings.ToLower(resource) {
	case "container", "pod", "image", "volume", "network":
		return "." + strings.ToLower(resource)
	default:
		return ".container"
	}
}
