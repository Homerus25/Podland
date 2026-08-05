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
	Get(ctx context.Context, userID, machineID, name string) (*QuadletGetResponse, error)
}

// ListFilters represents the filters for listing quadlets.
type ListFilters struct {
	Resource string
	Name     string
}

// QuadletLister defines the interface for listing quadlets.
type QuadletLister interface {
	QuadletList(ctx context.Context, opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error)
}

// QuadletPrinter defines the interface for getting quadlet content.
type QuadletPrinter interface {
	QuadletPrint(ctx context.Context, name string) (string, error)
}

// tunnelQuadletLister implements QuadletLister using tunnel.ContainerEngine.
type tunnelQuadletLister struct{}

func (t *tunnelQuadletLister) QuadletList(ctx context.Context, opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: ctx,
	}
	return containerEngine.QuadletList(ctx, opts)
}

// tunnelQuadletPrinter implements QuadletPrinter using tunnel.ContainerEngine.
type tunnelQuadletPrinter struct{}

func (t *tunnelQuadletPrinter) QuadletPrint(ctx context.Context, name string) (string, error) {
	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: ctx,
	}
	return containerEngine.QuadletPrint(ctx, name)
}

// QuadletService provides business logic for quadlets
type QuadletService struct {
	connManager    machines.ConnectionManagerInterface
	quadletLister  QuadletLister
	quadletPrinter QuadletPrinter
}

// NewQuadletService creates a new QuadletService with the given connection manager interface and quadlet lister.
func NewQuadletService(connManager machines.ConnectionManagerInterface, quadletLister QuadletLister, quadletPrinter QuadletPrinter) *QuadletService {
	return &QuadletService{
		connManager:    connManager,
		quadletLister:  quadletLister,
		quadletPrinter: quadletPrinter,
	}
}

// NewQuadletServiceWithConcreteDeps creates a new QuadletService with concrete dependencies.
// This maintains backward compatibility with existing code that uses *connections.ConnectionManager.
func NewQuadletServiceWithConcreteDeps(connManager *connections.ConnectionManager) *QuadletService {
	return NewQuadletService(&concreteConnectionManager{connManager: connManager}, &tunnelQuadletLister{}, &tunnelQuadletPrinter{})
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

	quadlets, err := s.quadletLister.QuadletList(connCtx, entities.QuadletListOptions{})
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

// Get returns a specific quadlet by name on the specified machine.
func (s *QuadletService) Get(ctx context.Context, userID, machineID, name string) (*QuadletGetResponse, error) {
	connCtxInterface, err := s.connManager.GetConnection(ctx, machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	content, err := s.quadletPrinter.QuadletPrint(connCtx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get quadlet content: %w", err)
	}

	quadlets, err := s.quadletLister.QuadletList(connCtx, entities.QuadletListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list quadlets: %w", err)
	}

	var targetQuadlet *entities.ListQuadlet
	for _, q := range quadlets {
		if q.Name == name {
			targetQuadlet = q
			break
		}
	}

	if targetQuadlet == nil {
		return nil, appErrors.ErrQuadletNotFound
	}

	listResponse := FromListQuadlet(targetQuadlet)
	return &QuadletGetResponse{
		ID:       listResponse.ID,
		Name:     listResponse.Name,
		Path:     listResponse.Path,
		Unit:     listResponse.Unit,
		Resource: listResponse.Resource,
		State:    listResponse.State,
		Content:  content,
	}, nil
}
