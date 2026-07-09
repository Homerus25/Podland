package containers

import (
	"context"
	"fmt"

	"github.com/yourorg/podland/backend/internal/connections"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
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

// ContainerServiceInterface defines the interface for container services
// This interface is used for dependency injection and testing
type ContainerServiceInterface interface {
	List(ctx context.Context, userID, machineID string, opts entities.ContainerListOptions) ([]entities.ListContainer, error)
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
	connCtxInterface, err := s.connManager.GetConnection(ctx, machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}

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
