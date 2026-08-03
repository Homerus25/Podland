package quadlets

import (
	"context"

	"github.com/yourorg/podland/backend/internal/connections"
	"github.com/yourorg/podland/backend/internal/domain/machines"
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
// Method signatures will be added as slices 1–9 are implemented.
type QuadletServiceInterface interface {
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
