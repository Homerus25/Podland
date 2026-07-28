package containers

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/yourorg/podland/backend/internal/connections"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
	"go.podman.io/podman/v6/pkg/specgen"
)

// concreteConnectionManager wraps a *connections.ConnectionManager to implement ConnectionManagerInterface
// This is needed for backward compatibility
type concreteConnectionManager struct {
	connManager *connections.ConnectionManager
}

// GetConnection implements ConnectionManagerInterface by calling the underlying ConnectionManager
func (c *concreteConnectionManager) GetConnection(ctx context.Context, machineID, userID string) (any, error) {
	return c.connManager.GetConnection(ctx, machineID, userID)
}

func (c *concreteConnectionManager) Release(machineID string) {
	c.connManager.Release(machineID)
}

// ContainerServiceInterface defines the interface for container services
// This interface is used for dependency injection and testing
type ContainerServiceInterface interface {
	List(ctx context.Context, userID, machineID string, opts entities.ContainerListOptions) ([]entities.ListContainer, error)
	Create(ctx context.Context, userID, machineID string, request ContainerCreateRequest) (*ContainerCreateResponse, error)
	Get(ctx context.Context, userID, machineID, containerID string) (*entities.ContainerInspectReport, error)
}

// ContainerService provides business logic for containers
type ContainerService struct {
	connManager machines.ConnectionManagerInterface
}

// NewContainerService creates a new ContainerService
func NewContainerService(connManager machines.ConnectionManagerInterface) *ContainerService {
	return &ContainerService{
		connManager: connManager,
	}
}

// NewContainerServiceWithConcreteDeps creates a new ContainerService with concrete dependencies
// This maintains backward compatibility with existing code that uses *connections.ConnectionManager
func NewContainerServiceWithConcreteDeps(connManager *connections.ConnectionManager) *ContainerService {
	return NewContainerService(&concreteConnectionManager{connManager: connManager})
}

// List returns a list of containers on the specified machine
func (s *ContainerService) List(ctx context.Context, userID, machineID string, opts entities.ContainerListOptions) ([]entities.ListContainer, error) {
	// Get connection context to the machine
	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}

	defer s.connManager.Release(machineID)

	// Convert interface to context.Context
	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	// Create container engine with the connection context
	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	// Call the official podman ContainerList method
	containers, err := containerEngine.ContainerList(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	return containers, nil
}

// Create creates a new container on the specified machine
func (s *ContainerService) Create(ctx context.Context, userID, machineID string, request ContainerCreateRequest) (*ContainerCreateResponse, error) {
	// Get connection context to the machine
	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	// Convert interface to context.Context
	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	// Create container engine with the connection context
	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	// Create a basic spec generator with the image
	spec := specgen.NewSpecGenerator(request.Image, false)

	// Set container name if provided
	if request.Name != "" {
		spec.Name = request.Name
	}

	// Set command if provided
	if len(request.Command) > 0 {
		spec.Command = request.Command
	}

	// Set working directory if provided
	if request.WorkingDir != "" {
		spec.WorkDir = request.WorkingDir
	}

	// Set environment variables if provided
	if len(request.Environment) > 0 {
		if spec.Env == nil {
			spec.Env = make(map[string]string)
		}
		maps.Copy(spec.Env, request.Environment)
	}

	// Set labels if provided
	if len(request.Labels) > 0 {
		if spec.Labels == nil {
			spec.Labels = make(map[string]string)
		}
		maps.Copy(spec.Labels, request.Labels)
	}

	// Note: For now, we're skipping port bindings, mounts, privileged mode,
	// restart policy, and resource limits due to API complexity.
	// These can be added in a future enhancement.

	// Create the container using Podman API
	createResponse, err := containerEngine.ContainerCreate(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Convert the response to our DTO format
	response := &ContainerCreateResponse{
		ID:        createResponse.Id,
		Name:      request.Name,
		State:     "created",
		Image:     request.Image,
		ImageID:   "", // ImageID is not available in ContainerCreateReport
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	return response, nil
}

// Get returns detailed information about a specific container
func (s *ContainerService) Get(ctx context.Context, userID, machineID, containerID string) (*entities.ContainerInspectReport, error) {
	// Get connection context to the machine
	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	// Convert interface to context.Context
	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	// Create container engine with the connection context
	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	// Call the official podman ContainerInspect method
	containerReports, _, err := containerEngine.ContainerInspect(ctx, []string{containerID}, entities.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	if len(containerReports) == 0 {
		return nil, fmt.Errorf("no container found with ID: %s", containerID)
	}

	return containerReports[0], nil
}
