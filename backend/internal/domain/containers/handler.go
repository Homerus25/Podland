package containers

import (
	"net/http"
)

// ContainerHandler handles container requests
type ContainerHandler struct {
	service        *ContainerService
	machineService *any
}

// NewContainerHandler creates a new ContainerHandler
func NewContainerHandler(service *ContainerService, machineService any) *ContainerHandler {
	return &ContainerHandler{
		service:        service,
		machineService: &machineService,
	}
}

// List handles listing containers
func (h *ContainerHandler) List(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Create handles creating a container
func (h *ContainerHandler) Create(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Get handles getting a container by ID
func (h *ContainerHandler) Get(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Start handles starting a container
func (h *ContainerHandler) Start(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Stop handles stopping a container
func (h *ContainerHandler) Stop(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Delete handles deleting a container
func (h *ContainerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
