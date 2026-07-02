package containers

import (
	"github.com/yourorg/podland/backend/internal/connections"
)

// ContainerService provides business logic for containers
type ContainerService struct {
	connManager *connections.ConnectionManager
}

// NewContainerService creates a new ContainerService
func NewContainerService(connManager *connections.ConnectionManager) *ContainerService {
	return &ContainerService{
		connManager: connManager,
	}
}
