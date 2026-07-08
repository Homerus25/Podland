package events

import (
	"net/http"
)

// EventHandler handles event streaming requests
type EventHandler struct {
	service        *EventService
	machineService any
	errorHandler   any
}

// NewEventHandler creates a new EventHandler
func NewEventHandler(service *EventService, machineService any, errorHandler any) *EventHandler {
	return &EventHandler{
		service:        service,
		machineService: machineService,
		errorHandler:   errorHandler,
	}
}

// HandleEvents handles SSE event streaming
func (h *EventHandler) HandleEvents(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
