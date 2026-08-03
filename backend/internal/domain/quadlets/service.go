package quadlets

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/podland/backend/internal/connections"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

var QuadletListFunc func(ctx context.Context, opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error)

// Compile-time check that *QuadletService satisfies QuadletServiceInterface.
var _ QuadletServiceInterface = (*QuadletService)(nil)

// concreteConnectionManager wraps a *connections.ConnectionManager to implement ConnectionManagerInterface.
// This is needed for backward compatibility.
type concreteConnectionManager struct {
	connManager *connections.ConnectionManager
}

// GetConnection implements ConnectionManagerInterface by calling the underlying ConnectionManager.
func (c *concreteConnectionManager) GetConnection(ctx context.Context, machineID, userID string) (any, error) {
	return c.connManager.GetConnection(ctx, machineID, userID)
}

func (c *concreteConnectionManager) Release(machineID string) {
	c.connManager.Release(machineID)
}

// QuadletServiceInterface defines the interface for quadlet services.
// This interface is used for dependency injection and testing.
type QuadletServiceInterface interface {
	List(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error)
}

// ListFilters represents the filters for listing quadlets.
type ListFilters struct {
	Resource string
	Name     string
}

// QuadletService provides business logic for quadlets
type QuadletService struct {
	connManager machines.ConnectionManagerInterface
}

// NewQuadletService creates a new QuadletService with the given connection manager interface.
func NewQuadletService(connManager machines.ConnectionManagerInterface) *QuadletService {
	return &QuadletService{
		connManager: connManager,
	}
}

// NewQuadletServiceWithConcreteDeps creates a new QuadletService with concrete dependencies.
// This maintains backward compatibility with existing code that uses *connections.ConnectionManager.
func NewQuadletServiceWithConcreteDeps(connManager *connections.ConnectionManager) *QuadletService {
	return NewQuadletService(&concreteConnectionManager{connManager: connManager})
}

// List returns a list of quadlets on the specified machine.
func (s *QuadletService) List(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error) {
	connCtxInterface, err := s.connManager.GetConnection(ctx, machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	var quadlets []*entities.ListQuadlet
	
	if QuadletListFunc != nil {
		quadlets, err = QuadletListFunc(connCtx, entities.QuadletListOptions{})
	} else {
		containerEngine := &tunnel.ContainerEngine{
			ClientCtx: connCtx,
		}
		quadlets, err = containerEngine.QuadletList(connCtx, entities.QuadletListOptions{})
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to list quadlets: %w", err)
	}

	result := ToQuadletListResponse(quadlets)

	result = applyFilters(result, filters)

	return result, nil
}

// applyFilters applies the resource and name filters to the quadlet list.
func applyFilters(quadlets []QuadletListResponse, filters ListFilters) []QuadletListResponse {
	if filters.Resource == "" && filters.Name == "" {
		return quadlets
	}

	var filtered []QuadletListResponse
	for _, quadlet := range quadlets {
		if filters.Resource != "" && quadlet.Resource != filters.Resource {
			continue
		}
		if filters.Name != "" && !strings.Contains(strings.ToLower(quadlet.Name), strings.ToLower(filters.Name)) {
			continue
		}
		filtered = append(filtered, quadlet)
	}

	return filtered
}
