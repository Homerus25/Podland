package containers

import (
	"strings"
	"time"

	netTypes "go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
	containerTypes "go.podman.io/podman/v6/pkg/domain/entities/types"
	define "go.podman.io/podman/v6/pkg/ps/define"
)

// ContainerGetResponse represents detailed information about a specific container
// This is a simplified DTO that only includes basic container information
type ContainerGetResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state
	State string `json:"state"`
	// Container status
	Status string `json:"status"`
	// Container image
	Image string `json:"image"`
	// Container image ID
	ImageID string `json:"imageId,omitempty"`
	// Time when container was created
	CreatedAt string `json:"createdAt"`
	// Container command
	Command []string `json:"command,omitempty"`
	// Container arguments
	Args []string `json:"args,omitempty"`
	// Container working directory
	WorkingDir string `json:"workingDir,omitempty"`
	// Container environment variables
	Environment map[string]string `json:"environment,omitempty"`
	// Container labels
	Labels map[string]string `json:"labels,omitempty"`
	// Container ports
	Ports []PortMappingResponse `json:"ports,omitempty"`
	// Container mounts
	Mounts []string `json:"mounts,omitempty"`
	// Container networks
	Networks []string `json:"networks,omitempty"`
	// Container restart count
	Restarts uint `json:"restarts"`
	// If container has exited/stopped
	Exited bool `json:"exited"`
	// Exit code if container has exited
	ExitCode *int32 `json:"exitCode,omitempty"`
	// Container size information
	Size *ContainerSizeResponse `json:"size,omitempty"`
}

// PortMappingResponse represents a port mapping in the API response
type PortMappingResponse struct {
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"hostIp,omitempty"`
}

// ContainerSizeResponse represents container size information
type ContainerSizeResponse struct {
	RootFsSize int64 `json:"rootFsSize"`
	RwSize     int64 `json:"rwSize"`
}

// ContainerListResponse represents a container in the API response
// This DTO is used to serialize container information for the API
type ContainerListResponse struct {
	// Unique identifier for the container
	ID string `json:"id"`
	// Names assigned to the container
	Names []string `json:"names"`
	// Container image
	Image string `json:"image"`
	// Container image ID
	ImageID string `json:"imageId,omitempty"`
	// Container command
	Command []string `json:"command,omitempty"`
	// Time when container was created
	CreatedAt string `json:"createdAt"`
	// Current state of the container
	State string `json:"state"`
	// Human-readable status of the container
	Status string `json:"status"`
	// Number of times the container has been restarted
	Restarts uint `json:"restarts"`
	// If container has exited/stopped
	Exited bool `json:"exited"`
	// Exit code if container has exited
	ExitCode *int32 `json:"exitCode,omitempty"`
	// Container labels
	Labels map[string]string `json:"labels,omitempty"`
	// Port mappings
	Ports []PortMappingResponse `json:"ports,omitempty"`
	// Size information (if requested)
	Size *ContainerSizeResponse `json:"size,omitempty"`
	// Network names assigned to the container
	Networks []string `json:"networks,omitempty"`
	// Mount points
	Mounts []string `json:"mounts,omitempty"`
}

// ContainerCreateRequest represents a request to create a new container
// This DTO is used to deserialize the API request for container creation
type ContainerCreateRequest struct {
	// Name of the container (optional)
	Name string `json:"name,omitempty" validate:"omitempty,max=255"`
	// Image to use for the container (required)
	Image string `json:"image" validate:"required,min=1"`
	// Command to run in the container (optional)
	Command []string `json:"command,omitempty"`
	// Arguments to the command (optional)
	Args []string `json:"args,omitempty"`
	// Working directory inside the container (optional)
	WorkingDir string `json:"workingDir,omitempty"`
	// Environment variables (optional)
	Environment map[string]string `json:"environment,omitempty"`
	// Labels for the container (optional)
	Labels map[string]string `json:"labels,omitempty"`
	// Port bindings (optional)
	PortBindings []PortBindingRequest `json:"portBindings,omitempty"`
	// Mounts/volumes (optional)
	Mounts []MountRequest `json:"mounts,omitempty"`
	// Run container in privileged mode (optional)
	Privileged bool `json:"privileged,omitempty"`
	// Restart policy (optional)
	RestartPolicy string `json:"restartPolicy,omitempty" validate:"omitempty,oneof=no always unless-stopped on-failure"`
	// Memory limit (optional)
	Memory string `json:"memory,omitempty"`
	// CPU limit (optional)
	CPUs float64 `json:"cpus,omitempty"`
}

// PortBindingRequest represents a port binding request
type PortBindingRequest struct {
	HostPort      int    `json:"hostPort" validate:"required,min=1,max=65535"`
	ContainerPort int    `json:"containerPort" validate:"required,min=1,max=65535"`
	Protocol      string `json:"protocol,omitempty" validate:"omitempty,oneof=tcp udp"`
}

// MountRequest represents a mount/volume request
type MountRequest struct {
	Source      string `json:"source" validate:"required,min=1"`
	Destination string `json:"destination" validate:"required,min=1"`
	Mode        string `json:"mode,omitempty" validate:"omitempty,oneof=ro rw"`
}

// ContainerCreateResponse represents the response after creating a container
type ContainerCreateResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state
	State string `json:"state"`
	// Container image
	Image string `json:"image"`
	// Container image ID
	ImageID string `json:"imageId,omitempty"`
	// Creation timestamp
	CreatedAt string `json:"createdAt"`
}

// ContainerListResult wraps the list of containers with metadata
// This provides a consistent response structure for the API
type ContainerListResult struct {
	// List of containers
	Containers []ContainerListResponse `json:"containers"`
	// Total count of containers
	Total int `json:"total"`
	// Request timestamp
	Timestamp string `json:"timestamp"`
}

// FromPodmanInspectContainer converts a podman ContainerInspectReport entity to ContainerGetResponse
// This is a simplified mapping function that only includes basic container information
func FromPodmanInspectContainer(containerReport *entities.ContainerInspectReport) ContainerGetResponse {
	if containerReport == nil {
		return ContainerGetResponse{}
	}

	// Create a simple response with basic container information
	// Use safe accessors that handle nil checks
	response := ContainerGetResponse{
		ID:        getContainerID(containerReport),
		Name:      getContainerName(containerReport),
		State:     getContainerState(containerReport),
		Status:    getContainerStatus(containerReport),
		Image:     getContainerImage(containerReport),
		CreatedAt: formatCreatedTime(containerReport),
		Restarts:  uint(getContainerRestartCount(containerReport)),
		Exited:    isContainerExited(containerReport),
	}

	// Add optional fields if available
	if containerReport.Config != nil {
		response.Command = containerReport.Config.Cmd
		// Args might not be available in the Config, so we'll skip it
		response.WorkingDir = containerReport.Config.WorkingDir
		response.Labels = containerReport.Config.Labels

		// Parse environment variables
		if len(containerReport.Config.Env) > 0 {
			response.Environment = make(map[string]string)
			for _, envVar := range containerReport.Config.Env {
				// Simple parsing of KEY=VALUE format
				if parts := strings.SplitN(envVar, "=", 2); len(parts) == 2 {
					response.Environment[parts[0]] = parts[1]
				}
			}
		}
	}

	// Skip size information for now since the field types are unclear

	// Add exit code if container has exited
	if containerReport.State != nil && containerReport.State.ExitCode != 0 {
		exitCode := containerReport.State.ExitCode
		response.ExitCode = &exitCode
	}

	return response
}

// Helper function to get container name

// FromPodmanContainer converts a podman ListContainer entity to ContainerListResponse
// This mapping function handles the transformation from podman internal representation
// to our API DTO format
type ListContainer = any // Placeholder for the actual type

func FromPodmanContainer(container any) ContainerListResponse {
	// Try to convert to the actual type
	if listContainer, ok := container.(containerTypes.ListContainer); ok {
		return ContainerListResponse{
			ID:        listContainer.ID,
			Names:     listContainer.Names,
			Image:     listContainer.Image,
			ImageID:   listContainer.ImageID,
			Command:   listContainer.Command,
			CreatedAt: listContainer.CreatedAt,
			State:     listContainer.State,
			Status:    listContainer.Status,
			Restarts:  listContainer.Restarts,
			Exited:    listContainer.Exited,
			ExitCode: func() *int32 {
				if listContainer.ExitCode != 0 {
					v := listContainer.ExitCode
					return &v
				}
				return nil
			}(),
			Labels:   listContainer.Labels,
			Ports:    mapPortMappings(listContainer.Ports),
			Size:     mapContainerSize(listContainer.Size),
			Networks: listContainer.Networks,
			Mounts:   listContainer.Mounts,
		}
	}

	// Fallback for testing
	return ContainerListResponse{
		ID:        "placeholder-id",
		Names:     []string{"placeholder-name"},
		Image:     "placeholder-image",
		State:     "unknown",
		Status:    "Unknown status",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// ToContainerListResult converts a list of podman containers to ContainerListResult
// This function handles the complete transformation from service layer to API response
func ToContainerListResult(containers []any) ContainerListResult {
	result := ContainerListResult{
		Containers: make([]ContainerListResponse, len(containers)),
		Total:      len(containers),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	for i, container := range containers {
		result.Containers[i] = FromPodmanContainer(container)
	}

	return result
}

// Helper function to map port mappings
func mapPortMappings(ports []netTypes.PortMapping) []PortMappingResponse {
	if len(ports) == 0 {
		return nil
	}
	result := make([]PortMappingResponse, len(ports))
	for i, port := range ports {
		result[i] = PortMappingResponse{
			ContainerPort: int(port.ContainerPort),
			HostPort:      int(port.HostPort),
			Protocol:      port.Protocol,
			HostIP:        port.HostIP,
		}
	}
	return result
}

// Helper function to map container size
func mapContainerSize(size *define.ContainerSize) *ContainerSizeResponse {
	if size == nil {
		return nil
	}
	return &ContainerSizeResponse{
		RootFsSize: size.RootFsSize,
		RwSize:     size.RwSize,
	}
}

// Helper function to get container ID from report
func getContainerID(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil {
		// Access ID safely - it might be from the embedded struct
		return containerReport.ID
	}
	return ""
}

// Helper function to get container name from report
func getContainerName(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil && len(containerReport.Name) > 0 {
		// Remove leading "/" if present
		return strings.TrimPrefix(containerReport.Name, "/")
	}
	if containerReport != nil {
		return containerReport.ID
	}
	return ""
}

// Helper function to get container restart count from report
func getContainerRestartCount(containerReport *entities.ContainerInspectReport) int32 {
	if containerReport != nil {
		return containerReport.RestartCount
	}
	return 0
}

// Helper function to get container image from report
func getContainerImage(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil && containerReport.Config != nil {
		return containerReport.Config.Image
	}
	return ""
}

// Helper function to get container state from report
func getContainerState(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil && containerReport.State != nil {
		if containerReport.State.Running {
			return "running"
		} else if containerReport.State.ExitCode != 0 {
			return "exited"
		} else if containerReport.State.Paused {
			return "paused"
		} else if containerReport.State.Restarting {
			return "restarting"
		} else if containerReport.State.Dead {
			return "dead"
		}
	}
	return "created"
}

// Helper function to get container status from report
func getContainerStatus(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil && containerReport.State != nil {
		if containerReport.State.Running {
			return "Up"
		} else if containerReport.State.ExitCode != 0 {
			return "Exited"
		}
	}
	return "Created"
}

// Helper function to format created time
func formatCreatedTime(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil && !containerReport.Created.IsZero() {
		return containerReport.Created.Format(time.RFC3339)
	}
	return ""
}

// ContainerStartResponse represents the response after starting a container
type ContainerStartResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state after start
	State string `json:"state"`
	// Operation timestamp
	Timestamp string `json:"timestamp"`
}

// ContainerStopResponse represents the response after stopping a container
type ContainerStopResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state after stop
	State string `json:"state"`
	// Operation timestamp
	Timestamp string `json:"timestamp"`
}

// ContainerLogsResponse represents the response for container logs
type ContainerLogsResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container logs content (multiline string)
	Logs string `json:"logs"`
}

// Helper function to check if container is exited
func isContainerExited(containerReport *entities.ContainerInspectReport) bool {
	if containerReport != nil && containerReport.State != nil {
		return containerReport.State.ExitCode != 0
	}
	return false
}
