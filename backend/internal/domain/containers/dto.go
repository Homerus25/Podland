package containers

import (
	"time"

	"go.podman.io/podman/v6/pkg/domain/entities/types"
)

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

// FromPodmanContainer converts a podman ListContainer entity to ContainerListResponse
// This mapping function handles the transformation from podman internal representation
// to our API DTO format
func FromPodmanContainer(container types.ListContainer) ContainerListResponse {
	// Handle optional exit code
	var exitCode *int32
	if container.Exited && container.ExitCode != 0 {
		exitCode = &container.ExitCode
	}

	// Map ports
	ports := make([]PortMappingResponse, len(container.Ports))
	for i, port := range container.Ports {
		ports[i] = PortMappingResponse{
			ContainerPort: int(port.ContainerPort),
			HostPort:      int(port.HostPort),
			Protocol:      port.Protocol,
			HostIP:        port.HostIP,
		}
	}

	// Map size information if available
	var size *ContainerSizeResponse
	if container.Size != nil {
		size = &ContainerSizeResponse{
			RootFsSize: container.Size.RootFsSize,
			RwSize:     container.Size.RwSize,
		}
	}

	// Format created time
	createdAt := container.Created.Format(time.RFC3339)
	if container.CreatedAt != "" {
		createdAt = container.CreatedAt
	}

	return ContainerListResponse{
		ID:        container.ID,
		Names:     container.Names,
		Image:     container.Image,
		ImageID:   container.ImageID,
		Command:   container.Command,
		CreatedAt: createdAt,
		State:     container.State,
		Status:    container.Status,
		Restarts:  container.Restarts,
		Exited:    container.Exited,
		ExitCode:  exitCode,
		Labels:    container.Labels,
		Ports:     ports,
		Size:      size,
		Networks:  container.Networks,
		Mounts:    container.Mounts,
	}
}

// ToContainerListResult converts a list of podman containers to ContainerListResult
// This function handles the complete transformation from service layer to API response
func ToContainerListResult(containers []types.ListContainer) ContainerListResult {
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