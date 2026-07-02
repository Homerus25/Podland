package events

import (
	"github.com/yourorg/podland/backend/internal/connections"
)

// EventService provides business logic for event streaming
type EventService struct {
	connManager *connections.ConnectionManager
}

// NewEventService creates a new EventService
func NewEventService(connManager *connections.ConnectionManager) *EventService {
	return &EventService{
		connManager: connManager,
	}
}