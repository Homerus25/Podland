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

// QuadletGenerateRequest represents a request to generate a quadlet from an existing
// container or pod on the remote machine.
type QuadletGenerateRequest struct {
	// ID of the container or pod to generate a quadlet from. Required.
	ResourceID string `json:"resourceId" validate:"required,min=1"`
	// Type of resource to generate from. Must be either "container" or "pod". Required.
	ResourceType string `json:"resourceType" validate:"required,oneof=container pod"`
}

// QuadletGenerateResponse represents the response from generating a quadlet.
// The response includes the generated quadlet content with state "unknown" since
// the quadlet is not installed or started by this endpoint.
type QuadletGenerateResponse struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// Name of the quadlet
	Name string `json:"name"`
	// Path to the quadlet file (empty since not installed)
	Path string `json:"path,omitempty"`
	// Systemd unit name (empty since not installed)
	Unit string `json:"unit,omitempty"`
	// Resource type (container or pod)
	Resource string `json:"resource"`
	// State of the quadlet (always "unknown" for generated quadlets)
	State string `json:"state"`
	// Generated quadlet file content
	Content string `json:"content"`
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

// QuadletLogsResponse represents the response for quadlet logs (non-streaming)
type QuadletLogsResponse struct {
	// Quadlet ID (the quadlet name)
	ID string `json:"id"`
	// Quadlet logs content (multiline string)
	Logs string `json:"logs"`
}

// QuadletLogEvent represents a single log line in streaming mode
type QuadletLogEvent struct {
	// ISO8601 timestamp
	Timestamp string `json:"timestamp"`
	// Log level (INFO, WARN, ERROR, etc.)
	Level string `json:"level"`
	// Log message content
	Message string `json:"message"`
}

// resolveManagedContainerName resolves the managed container name for a quadlet.
// For container-type quadlets, the managed container is the systemd-spawned
// container, which podman names `systemd-<basename>` — the quadlet file name
// without its resource extension (podman quadlet generator: `--name systemd-%N`).
// For pod-type quadlets, we use the Pod container name from the ListQuadlet result.
// An explicit `ContainerName=` key in the quadlet content overrides this
// convention (see explicitContainerNameFromQuadletContent); callers that have
// the content at hand should check that first.
func resolveManagedContainerName(quadlet *entities.ListQuadlet, quadletName string) string {
	if quadlet == nil {
		return quadletName
	}

	// For pod-type quadlets, use the Pod container name if available
	if quadlet.Pod != "" {
		return quadlet.Pod
	}

	// For container-type quadlets and others, derive the systemd-spawned
	// container name: strip the resource extension ("foo.container" → "foo")
	// and apply podman quadlet's "systemd-" naming convention.
	base := quadletName
	if idx := strings.LastIndex(quadletName, "."); idx > 0 {
		base = quadletName[:idx]
	}
	return "systemd-" + base
}

// explicitContainerNameFromQuadletContent extracts the value of the
// `ContainerName=` key from quadlet file content. Podman's quadlet generator
// passes it to `podman run --name`, so when present the managed container is
// named verbatim instead of the default `systemd-<basename>`. Returns "" when
// the key is absent (or has an empty value), meaning the default applies.
// Key matching is case-insensitive, matching systemd's unit-file parser; the
// value is used verbatim apart from surrounding whitespace and quotes.
func explicitContainerNameFromQuadletContent(content string) string {
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "ContainerName") {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if value != "" {
			return value
		}
		return ""
	}
	return ""
}

// QuadletImportRequest represents a request to import quadlets from uploaded content.
// Each entry contains the quadlet name, resource type, and file content.
type QuadletImportRequest struct {
	// List of quadlets to import. At least one entry is required.
	Quadlets []QuadletImportEntry `json:"quadlets" validate:"required,min=1,dive"`
}

// QuadletImportEntry represents a single quadlet to import.
type QuadletImportEntry struct {
	// Name of the quadlet (without extension). Required.
	Name string `json:"name" validate:"required,min=1"`
	// Resource type. Must be one of: container, pod, image, volume, network. Required.
	Resource string `json:"resource" validate:"required,oneof=container pod image volume network"`
	// Raw quadlet file content. Required.
	Content string `json:"content" validate:"required,min=1"`
}

// QuadletImportResponse represents the response from importing quadlets.
// Returns separate arrays for successfully imported and failed quadlets.
type QuadletImportResponse struct {
	// Successfully imported quadlets
	Imported []QuadletImportResult `json:"imported"`
	// Failed imports with error details
	Failed []QuadletImportError `json:"failed"`
}

// QuadletImportResult represents a successfully imported quadlet.
type QuadletImportResult struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// Name of the quadlet
	Name string `json:"name"`
	// Path to the installed quadlet file
	Path string `json:"path"`
	// State of the quadlet (reported as "unknown" after import)
	State string `json:"state"`
}

// QuadletImportError represents a failed quadlet import with error details.
type QuadletImportError struct {
	// Name of the quadlet that failed to import
	Name string `json:"name"`
	// Error message explaining why the import failed
	Error string `json:"error"`
}
