package quadlets

import (
	"net/http"
)

// QuadletHandler handles quadlet requests
type QuadletHandler struct {
	service        *QuadletService
	machineService any
}

// NewQuadletHandler creates a new QuadletHandler
func NewQuadletHandler(service *QuadletService, machineService any) *QuadletHandler {
	return &QuadletHandler{
		service:        service,
		machineService: machineService,
	}
}

// List handles listing quadlets
func (h *QuadletHandler) List(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Create handles creating a quadlet
func (h *QuadletHandler) Create(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Get handles getting a quadlet by ID
func (h *QuadletHandler) Get(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Update handles updating a quadlet
func (h *QuadletHandler) Update(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Delete handles deleting a quadlet
func (h *QuadletHandler) Delete(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Start handles starting a quadlet
func (h *QuadletHandler) Start(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Stop handles stopping a quadlet
func (h *QuadletHandler) Stop(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Restart handles restarting a quadlet
func (h *QuadletHandler) Restart(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// GetLogs handles getting quadlet logs
func (h *QuadletHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Generate handles generating a quadlet
func (h *QuadletHandler) Generate(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Import handles importing quadlets
func (h *QuadletHandler) Import(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
