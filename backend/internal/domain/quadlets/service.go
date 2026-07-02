package quadlets

import (
	"github.com/yourorg/podland/backend/internal/connections"
)

// QuadletService provides business logic for quadlets
type QuadletService struct {
	connManager *connections.ConnectionManager
}

// NewQuadletService creates a new QuadletService
func NewQuadletService(connManager *connections.ConnectionManager) *QuadletService {
	return &QuadletService{
		connManager: connManager,
	}
}
